package interfaceshttpinteraction

type postActionRequest struct {
	PostID int64 `json:"post_id" binding:"required,gt=0"`
}
type postCommentCreateRequest struct {
	PostID  int64  `json:"post_id" binding:"required,gt=0"`
	Content string `json:"content" binding:"required,max=1000"`
}
type postCommentListRequest struct {
	PostID int64 `json:"post_id" binding:"required,gt=0"`
	Limit  int   `json:"limit" binding:"omitempty,min=1,max=100"`
	Offset int   `json:"offset" binding:"omitempty,min=0"`
}
type postCommentDeleteRequest struct {
	CommentID int64 `json:"comment_id" binding:"required,gt=0"`
}
type postInteractionIdempotencyHeader struct {
	IdempotencyKey string `header:"Idempotency-Key" binding:"omitempty,max=128"`
}
type postActionResponse struct {
	PostID        int64  `json:"post_id"`
	ActionType    string `json:"action_type"`
	Active        bool   `json:"active"`
	LikeCount     int    `json:"like_count"`
	FavoriteCount int    `json:"favorite_count"`
}
type postCommentResponse struct {
	ID            int64  `json:"id"`
	PostID        int64  `json:"post_id"`
	UserID        int64  `json:"user_id"`
	UserNickname  string `json:"user_nickname"`
	UserAvatarURL string `json:"user_avatar_url"`
	Content       string `json:"content"`
	CreatedAt     int64  `json:"created_at"`
	CommentCount  int    `json:"comment_count,omitempty"`
}
type postCommentListResponse struct {
	Items  []postCommentResponse `json:"items"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}
type deleteCommentResponse struct {
	CommentID    int64 `json:"comment_id"`
	Status       int   `json:"status"`
	CommentCount int   `json:"comment_count"`
}
