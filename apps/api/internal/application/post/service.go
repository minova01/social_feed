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

type Service struct {
	repo       domainpost.Repository
	publisher  PublishedEventPublisher
	statReader StatReader
}

type StatReader interface {
	BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error)
}

type PublishedEventPublisher interface {
	PublishPostPublished(ctx context.Context, event *PublishedEvent) error
}

type Option func(*Service)

type CreateResult struct {
	Post    *domainpost.Post
	Created bool
}

func New(repo domainpost.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithPublishedEventPublisher(publisher PublishedEventPublisher) Option {
	return func(service *Service) {
		service.publisher = publisher
	}
}

func WithStatReader(reader StatReader) Option {
	return func(service *Service) {
		service.statReader = reader
	}
}

func (s *Service) CreatePublished(ctx context.Context, authorID int64, content string, imageURLs []string) (*CreateResult, error) {
	post := domainpost.NewPublished(authorID, content, imageURLs, "")
	if err := s.repo.Save(ctx, post); err != nil {
		return nil, ErrSavePostFailed
	}
	s.publishCreatedPost(ctx, post)
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
	s.publishCreatedPost(ctx, post)
	return &CreateResult{Post: post, Created: true}, nil
}

func (s *Service) publishCreatedPost(ctx context.Context, post *domainpost.Post) {
	if s.publisher == nil {
		return
	}
	event := NewPublishedEvent(post.ID)
	if err := s.publisher.PublishPostPublished(ctx, event); err != nil {
		log.Printf("publish post.published failed: post_id=%d error=%v", post.ID, err)
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
