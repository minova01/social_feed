package domainpost

import "time"

const (
	StatusPublished = 2
	StatusOffline   = 3
	StatusDeleted   = 4

	BigCreatorFollowerThreshold = 10000
)

const (
	EventPublished = "post.published"
)

type TimelineItem struct {
	PostID      int64 `json:"post_id"`
	AuthorID    int64 `json:"author_id"`
	PublishedAt int64 `json:"published_at"`
}

type Post struct {
	ID             int64
	AuthorID       int64
	Content        string
	ImageURLs      []string
	Status         int
	LikeCount      int
	CommentCount   int
	FavoriteCount  int
	PublishedAt    int64
	CreatedAt      int64
	UpdatedAt      int64
	IdempotencyKey string
}

func NewPublished(authorID int64, content string, imageURLs []string, idempotencyKey string) *Post {
	now := time.Now().UnixMilli()
	return &Post{
		AuthorID:       authorID,
		Content:        content,
		ImageURLs:      append([]string(nil), imageURLs...),
		Status:         StatusPublished,
		PublishedAt:    now,
		IdempotencyKey: idempotencyKey,
	}
}

func Restore(id int64, authorID int64, content string, imageURLs []string, status int, likeCount int, commentCount int, favoriteCount int, publishedAt int64, createdAt int64, updatedAt int64, idempotencyKey string) *Post {
	return &Post{
		ID:             id,
		AuthorID:       authorID,
		Content:        content,
		ImageURLs:      append([]string(nil), imageURLs...),
		Status:         status,
		LikeCount:      likeCount,
		CommentCount:   commentCount,
		FavoriteCount:  favoriteCount,
		PublishedAt:    publishedAt,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		IdempotencyKey: idempotencyKey,
	}
}

func (p *Post) DeleteBy(authorID int64) error {
	if p.AuthorID != authorID {
		return ErrPostPermissionDenied
	}
	if p.Status != StatusDeleted {
		p.Status = StatusDeleted
		p.UpdatedAt = time.Now().UnixMilli()
	}
	return nil
}
