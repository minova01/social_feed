package applicationpost

type PostEvent struct {
	PostID int64 `json:"post_id"`
}

func NewPostEvent(postID int64) *PostEvent {
	return &PostEvent{PostID: postID}
}
