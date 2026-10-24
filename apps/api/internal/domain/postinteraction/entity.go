package domainpostinteraction

const (
	ActionTypeLike     = "LIKE"
	ActionTypeFavorite = "FAVORITE"

	ActionStatusActive   = 1
	ActionStatusCanceled = 2

	CommentStatusNormal  = 1
	CommentStatusDeleted = 2
)

type Action struct {
	ID             int64
	UserID         int64
	PostID         int64
	ActionType     string
	Status         int
	IdempotencyKey string
	CreatedAt      int64
	UpdatedAt      int64
}

func (a *Action) Active() bool {
	return a.Status == ActionStatusActive
}

type Comment struct {
	ID             int64
	PostID         int64
	UserID         int64
	UserNickname   string
	UserAvatarURL  string
	Content        string
	Status         int
	IdempotencyKey string
	CreatedAt      int64
	UpdatedAt      int64
}

type UserProfile struct {
	ID        int64
	Nickname  string
	AvatarURL string
}
