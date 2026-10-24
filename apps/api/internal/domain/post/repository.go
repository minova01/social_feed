package domainpost

import "context"

type Repository interface {
	Save(ctx context.Context, post *Post) error
	FindByID(ctx context.Context, id int64) (*Post, error)
	FindByIDAnyStatus(ctx context.Context, id int64) (*Post, error)
	FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*Post, error)
	ListByAuthor(ctx context.Context, authorID int64, limit int, offset int) ([]*Post, error)
	BatchGetPublicFeedItems(ctx context.Context, viewerID int64, postIDs []int64) (map[int64]*FeedItem, error)
	UpdateStatus(ctx context.Context, post *Post) error
}
