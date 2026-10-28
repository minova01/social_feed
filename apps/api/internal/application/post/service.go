package applicationpost

import (
	"context"
	"errors"
	"log"
	domainpost "social/internal/domain/post"
)

var ErrLoadPostFailed = errors.New("failed to load post")
var ErrSavePostFailed = errors.New("failed to save post")
var ErrUpdatePostFailed = errors.New("failed to update post")
var ErrSearchPostsFailed = errors.New("failed to search posts")

type Service struct {
	repo       domainpost.Repository
	publisher  PostEventPublisher
	statReader StatReader
	search     SearchIndex
}

type SearchIndex interface {
	SearchPostIDs(ctx context.Context, keyword string, limit int, offset int) ([]int64, error)
}

type PostEventPublisher interface {
	Publish(ctx context.Context, eventType string, postID int64) error
}

type StatReader interface {
	BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error)
}

type Option func(*Service)

type CreateResult struct {
	Post    *domainpost.Post
	Created bool
}

func WithPostEventPublisher(publisher PostEventPublisher) Option {
	return func(service *Service) {
		service.publisher = publisher
	}
}

func New(repo domainpost.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithStatReader(reader StatReader) Option {
	return func(service *Service) {
		service.statReader = reader
	}
}

func WithSearchIndex(index SearchIndex) Option {
	return func(service *Service) {
		service.search = index
	}
}

func (s *Service) CreatePublished(ctx context.Context, authorID int64, content string, imageURLs []string) (*CreateResult, error) {
	post := domainpost.NewPublished(authorID, content, imageURLs, "")
	if err := s.repo.Save(ctx, post); err != nil {
		return nil, ErrSavePostFailed
	}
	s.publishPostEvent(ctx, domainpost.EventPublished, post.ID)
	return &CreateResult{Post: post, Created: true}, nil
}

func (s *Service) CreatePublishedIdempotently(ctx context.Context, authorID int64, content string, imageURLs []string, idempotencyKey string) (*CreateResult, error) {
	existing, err := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
	if err == nil {
		return &CreateResult{Post: existing}, nil
	}
	if !errors.Is(err, domainpost.ErrPostNotFound) {
		return nil, ErrLoadPostFailed
	}

	post := domainpost.NewPublished(authorID, content, imageURLs, idempotencyKey)
	if err := s.repo.Save(ctx, post); err != nil {
		if errors.Is(err, domainpost.ErrDuplicateIdempotencyKey) {
			existing, loadErr := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
			if loadErr == nil {
				return &CreateResult{Post: existing}, nil
			}
			return nil, ErrLoadPostFailed
		}
		return nil, ErrSavePostFailed
	}
	s.publishPostEvent(ctx, domainpost.EventPublished, post.ID)
	return &CreateResult{Post: post, Created: true}, nil
}

func (s *Service) publishPostEvent(ctx context.Context, eventType string, postID int64) {
	if s.publisher == nil {
		return
	}
	if err := s.publisher.Publish(ctx, eventType, postID); err != nil {
		log.Printf("publish %s failed: post_id=%d error=%v", eventType, postID, err)
	}
}

func (s *Service) Get(ctx context.Context, postID int64) (*domainpost.Post, error) {
	post, err := s.repo.FindByID(ctx, postID)
	if err != nil {
		if errors.Is(err, domainpost.ErrPostNotFound) {
			return nil, err
		}
		return nil, ErrLoadPostFailed
	}
	s.overlayStats(ctx, []*domainpost.Post{post})
	return post, nil
}

func (s *Service) ListByAuthor(ctx context.Context, authorID int64, limit int, offset int) ([]*domainpost.Post, error) {
	posts, err := s.repo.ListByAuthor(ctx, authorID, limit, offset)
	if err != nil {
		return nil, ErrLoadPostFailed
	}
	s.overlayStats(ctx, posts)
	return posts, nil
}

func (s *Service) Search(ctx context.Context, viewerID int64, keyword string, limit int, offset int) ([]*domainpost.FeedItem, error) {
	postIDs, err := s.search.SearchPostIDs(ctx, keyword, limit, offset)
	if err != nil {
		return nil, ErrSearchPostsFailed
	}
	itemsByID, err := s.repo.BatchGetPublicFeedItems(ctx, viewerID, postIDs)
	if err != nil {
		return nil, ErrSearchPostsFailed
	}
	if s.statReader != nil {
		fallbacks := make(map[int64]*domainpost.Stat, len(itemsByID))
		for postID, item := range itemsByID {
			fallbacks[postID] = &domainpost.Stat{PostID: postID, LikeCount: item.LikeCount, FavoriteCount: item.FavoriteCount}
		}
		if stats, statErr := s.statReader.BatchGetPostStats(ctx, postIDs, fallbacks); statErr == nil {
			for postID, stat := range stats {
				if item := itemsByID[postID]; item != nil && stat != nil {
					item.LikeCount = stat.LikeCount
					item.FavoriteCount = stat.FavoriteCount
				}
			}
		}
	}
	items := make([]*domainpost.FeedItem, 0, len(postIDs))
	for _, postID := range postIDs {
		if item := itemsByID[postID]; item != nil {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Service) overlayStats(ctx context.Context, posts []*domainpost.Post) {
	if s.statReader == nil || len(posts) == 0 {
		return
	}
	postIDs := make([]int64, 0, len(posts))
	fallbacks := make(map[int64]*domainpost.Stat, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
		fallbacks[post.ID] = &domainpost.Stat{PostID: post.ID, LikeCount: post.LikeCount, FavoriteCount: post.FavoriteCount}
	}
	stats, err := s.statReader.BatchGetPostStats(ctx, postIDs, fallbacks)
	if err != nil {
		return
	}
	for _, post := range posts {
		if stat := stats[post.ID]; stat != nil {
			post.LikeCount = stat.LikeCount
			post.FavoriteCount = stat.FavoriteCount
		}
	}
}

func (s *Service) Delete(ctx context.Context, authorID int64, postID int64) error {
	post, err := s.repo.FindByIDAnyStatus(ctx, postID)
	if err != nil {
		if errors.Is(err, domainpost.ErrPostNotFound) {
			return err
		}
		return ErrLoadPostFailed
	}
	alreadyDeleted := post.Status == domainpost.StatusDeleted
	if err := post.DeleteBy(authorID); err != nil {
		return err
	}
	if alreadyDeleted {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, post); err != nil {
		if errors.Is(err, domainpost.ErrPostNotFound) {
			return err
		}
		return ErrUpdatePostFailed
	}
	return nil
}
