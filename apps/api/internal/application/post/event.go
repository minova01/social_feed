package applicationpost

type PublishedEvent struct {
	PostID int64 `json:"post_id"`
}

func NewPublishedEvent(postID int64) *PublishedEvent {
	return &PublishedEvent{PostID: postID}
}
