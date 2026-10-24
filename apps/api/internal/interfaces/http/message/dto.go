package interfaceshttpmessage

type createMessageRequest struct {
	UserID         int64  `json:"user_id" binding:"required,gt=0"`
	Type           string `json:"type" binding:"required,max=32"`
	Title          string `json:"title" binding:"required,max=128"`
	Content        string `json:"content" binding:"max=1024"`
	EventID        string `json:"event_id" binding:"omitempty,max=64"`
	ActorID        int64  `json:"actor_id"`
	ActorNickname  string `json:"actor_nickname" binding:"omitempty,max=40"`
	ActorAvatarURL string `json:"actor_avatar_url" binding:"omitempty,max=512"`
}

type messageListRequest struct {
	Cursor string `json:"cursor" binding:"omitempty,max=512"`
	Limit  int    `json:"limit" binding:"omitempty,min=1,max=100"`
}

type markReadRequest struct {
	MessageIDs []int64 `json:"message_ids" binding:"required,min=1,max=100,dive,gt=0"`
}

type messageResponse struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	EventID        string `json:"event_id,omitempty"`
	ActorID        int64  `json:"actor_id,omitempty"`
	ActorNickname  string `json:"actor_nickname,omitempty"`
	ActorAvatarURL string `json:"actor_avatar_url,omitempty"`
	IsRead         bool   `json:"is_read"`
	CreatedAt      int64  `json:"created_at"`
	ReadAt         *int64 `json:"read_at,omitempty"`
}

type messageListResponse struct {
	Items      []messageResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

type unreadStatResponse struct {
	UnreadCount int `json:"unread_count"`
}

type markReadResponse struct {
	UpdatedCount int `json:"updated_count"`
}
