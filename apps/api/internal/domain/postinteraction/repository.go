package domainpostinteraction

import "context"

type Repository interface {
	GetActionStat(ctx context.Context, postID int64) (likeCount int, favoriteCount int, err error)
	SetAction(ctx context.Context, userID int64, postID int64, actionType string, active bool, idempotencyKey string) (*Action, int, error)
	CreateComment(ctx context.Context, comment *Comment) (*Comment, int, int, error)
	FindCommentByUserAndIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*Comment, int, error)
	ListComments(ctx context.Context, postID int64, limit int, offset int) ([]*Comment, error)
	DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*Comment, int, int, error)
	GetPostAuthorID(ctx context.Context, postID int64) (int64, error)
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)
}
