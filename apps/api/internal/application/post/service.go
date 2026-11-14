package applicationpost

import (
	"context"
	"errors"
	"log"
	domainpost "social/internal/domain/post"
	"time"
)

var ErrLoadPostFailed = errors.New("failed to load post")
var ErrSavePostFailed = errors.New("failed to save post")
var ErrUpdatePostFailed = errors.New("failed to update post")
var ErrSearchPostsFailed = errors.New("failed to search posts")
var ErrPostRateLimited = errors.New("post rate limited")

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return ErrPostRateLimited.Error()
}

func (e *RateLimitError) Unwrap() error {
	return ErrPostRateLimited
}

type Service struct {
	repo        domainpost.Repository
	statReader  StatReader
	search      SearchIndex
	rateLimiter PostRateLimiter
}

type PostRateLimiter interface {
	AllowPost(ctx context.Context, authorID int64, idempotencyKey string) (bool, time.Duration, error)
}

type SearchIndex interface {
	SearchPostIDs(ctx context.Context, keyword string, limit int, offset int) ([]int64, error)
}

type StatReader interface {
	BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error)
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

func WithPostRateLimiter(limiter PostRateLimiter) Option {
	return func(service *Service) {
		service.rateLimiter = limiter
	}
}

func (s *Service) CreatePublished(ctx context.Context, authorID int64, content string, imageURLs []string) (*CreateResult, error) {
	if err := s.checkPostRateLimit(ctx, authorID, ""); err != nil {
		return nil, err
	}
	post := domainpost.NewPublished(authorID, content, imageURLs, "")
	if err := s.repo.Save(ctx, post); err != nil {
		return nil, ErrSavePostFailed
	}
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
	if err := s.checkPostRateLimit(ctx, authorID, idempotencyKey); err != nil {
		return nil, err
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
	return &CreateResult{Post: post, Created: true}, nil
}

func (s *Service) checkPostRateLimit(ctx context.Context, authorID int64, idempotencyKey string) error {
	if s.rateLimiter == nil {
		return nil
	}
	allowed, retryAfter, err := s.rateLimiter.AllowPost(ctx, authorID, idempotencyKey)
	if err != nil {
		log.Printf("check post rate limit failed: author_id=%d error=%v", authorID, err)
		return nil
	}
	if !allowed {
		return &RateLimitError{RetryAfter: retryAfter}
	}
	return nil
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
