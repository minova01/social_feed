package interfaceshttppost

type CreatePostRequest struct {
	Content   string   `json:"content" binding:"required,max=2000"`
	ImageURLs []string `json:"image_urls" binding:"max=9,dive,required,max=512,startswith=/uploads/posts/,excludes=.."`
}

type idempotencyHeader struct {
	IdempotencyKey string `header:"Idempotency-Key" binding:"omitempty,max=128"`
}

type postIDRequest struct {
	PostID int64 `json:"post_id" binding:"required,gt=0"`
}

type postListRequest struct {
	AuthorID int64 `json:"author_id" binding:"required,gt=0"`
	Limit    int   `json:"limit" binding:"omitempty,min=1,max=100"`
	Offset   int   `json:"offset" binding:"omitempty,min=0"`
}

type myPostListRequest struct {
	Limit  int `json:"limit" binding:"omitempty,min=1,max=100"`
	Offset int `json:"offset" binding:"omitempty,min=0"`
}

type searchPostRequest struct {
	Keyword string `json:"keyword" binding:"required,max=100"`
	Limit   int    `json:"limit" binding:"omitempty,min=1,max=50"`
	Offset  int    `json:"offset" binding:"omitempty,min=0,max=10000"`
}

type searchPostItemResponse struct {
	PostID          int64    `json:"post_id"`
	AuthorID        int64    `json:"author_id"`
	AuthorNickname  string   `json:"author_nickname"`
	AuthorAvatarURL string   `json:"author_avatar_url"`
	Content         string   `json:"content"`
	ImageURLs       []string `json:"image_urls"`
	LikeCount       int      `json:"like_count"`
	CommentCount    int      `json:"comment_count"`
	FavoriteCount   int      `json:"favorite_count"`
	Liked           bool     `json:"liked"`
	Favorited       bool     `json:"favorited"`
	PublishedAt     int64    `json:"published_at"`
}

type searchPostResponse struct {
	Items  []searchPostItemResponse `json:"items"`
	Limit  int                      `json:"limit"`
	Offset int                      `json:"offset"`
}

type postResponse struct {
	ID            int64    `json:"id"`
	AuthorID      int64    `json:"author_id"`
	Content       string   `json:"content"`
	ImageURLs     []string `json:"image_urls"`
	Status        int      `json:"status"`
	LikeCount     int      `json:"like_count"`
	CommentCount  int      `json:"comment_count"`
	FavoriteCount int      `json:"favorite_count"`
	PublishedAt   int64    `json:"published_at"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

type postListResponse struct {
	Items  []postResponse `json:"items"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}
