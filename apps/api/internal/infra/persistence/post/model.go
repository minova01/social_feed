package infrapost

type PostModel struct {
	ID             int64    `gorm:"column:id;primaryKey;autoIncrement"`
	AuthorID       int64    `gorm:"column:author_id;not null;index:idx_post_author_status_published,priority:1;uniqueIndex:uk_post_author_idempotency,priority:1"`
	Content        string   `gorm:"column:content;size:2000;not null"`
	ImageURLs      []string `gorm:"column:image_urls;type:json;not null;serializer:json"`
	Status         int      `gorm:"column:status;type:tinyint;not null;default:2;index:idx_post_author_status_published,priority:2;index:idx_post_status_published,priority:1"`
	LikeCount      int      `gorm:"column:like_count;not null;default:0"`
	CommentCount   int      `gorm:"column:comment_count;not null;default:0"`
	FavoriteCount  int      `gorm:"column:favorite_count;not null;default:0"`
	PublishedAt    int64    `gorm:"column:published_at;not null;index:idx_post_author_status_published,priority:3,sort:desc;index:idx_post_status_published,priority:2,sort:desc"`
	IdempotencyKey *string  `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_post_author_idempotency,priority:2"`
	CreatedAt      int64    `gorm:"column:created_at;not null;autoCreateTime:milli"`
	UpdatedAt      int64    `gorm:"column:updated_at;not null;autoUpdateTime:milli"`
}

func (PostModel) TableName() string {
	return "post"
}
