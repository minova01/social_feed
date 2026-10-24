package domainpost

type TimelineCursor struct {
	PublishedAt int64 `json:"published_at"`
	PostID      int64 `json:"post_id"`
}

type FeedState struct {
	UserID         int64
	LastSeenAt     int64
	LastSeenPostID int64
	CreatedAt      int64
	UpdatedAt      int64
}

type FeedItem struct {
	PostID          int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Content         string
	ImageURLs       []string
	LikeCount       int
	CommentCount    int
	FavoriteCount   int
	Liked           bool
	Favorited       bool
	PublishedAt     int64
}

type TimelineBatch struct {
	Items  []*TimelineItem
	Capped bool
}

type FeedRemainder struct {
	Items         []*TimelineItem `json:"items"`
	SafeWatermark TimelineCursor  `json:"safe_watermark"`
}

type HotItem struct {
	PostID   int64
	HotScore int
}

type Stat struct {
	PostID        int64
	LikeCount     int
	FavoriteCount int
}
