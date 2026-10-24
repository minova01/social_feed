package interfaceshttpfeed

type followingPostRequest struct {
	Limit int `json:"limit" binding:"omitempty,min=1,max=20"`
}

type hotPostRequest struct {
	Limit  int `json:"limit" binding:"omitempty,min=1,max=20"`
	Offset int `json:"offset" binding:"omitempty,min=0"`
}

type followingPostResponse struct {
	Items []followingPostItemResponse `json:"items"`
}

type followingPostItemResponse struct {
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
