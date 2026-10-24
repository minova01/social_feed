package infrainteraction

type PostActionModel struct {
	ID             int64   `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64   `gorm:"column:user_id;not null;uniqueIndex:uk_post_action_user_post_type,priority:1;index:idx_post_action_user_type_status,priority:1"`
	PostID         int64   `gorm:"column:post_id;not null;uniqueIndex:uk_post_action_user_post_type,priority:2;index:idx_post_action_post_type_status,priority:1"`
	ActionType     string  `gorm:"column:action_type;size:16;not null;uniqueIndex:uk_post_action_user_post_type,priority:3;index:idx_post_action_post_type_status,priority:2;index:idx_post_action_user_type_status,priority:2"`
	Status         int     `gorm:"column:status;type:tinyint;not null;default:1;index:idx_post_action_post_type_status,priority:3;index:idx_post_action_user_type_status,priority:3"`
	IdempotencyKey *string `gorm:"column:idempotency_key;size:128"`
	CreatedAt      int64   `gorm:"column:created_at;not null;autoCreateTime:milli"`
	UpdatedAt      int64   `gorm:"column:updated_at;not null;autoUpdateTime:milli"`
}

func (PostActionModel) TableName() string {
	return "post_action"
}

type PostCommentModel struct {
	ID             int64   `gorm:"column:id;primaryKey;autoIncrement"`
	PostID         int64   `gorm:"column:post_id;not null;index:idx_post_comment_post_status_created,priority:1"`
	UserID         int64   `gorm:"column:user_id;not null;index:idx_post_comment_user_created,priority:1;uniqueIndex:uk_post_comment_user_idempotency,priority:1"`
	Content        string  `gorm:"column:content;size:1000;not null"`
	Status         int     `gorm:"column:status;type:tinyint;not null;default:1;index:idx_post_comment_post_status_created,priority:2"`
	IdempotencyKey *string `gorm:"column:idempotency_key;size:128;uniqueIndex:uk_post_comment_user_idempotency,priority:2"`
	CreatedAt      int64   `gorm:"column:created_at;not null;autoCreateTime:milli;index:idx_post_comment_post_status_created,priority:3,sort:desc;index:idx_post_comment_user_created,priority:2,sort:desc"`
	UpdatedAt      int64   `gorm:"column:updated_at;not null;autoUpdateTime:milli"`
}

func (PostCommentModel) TableName() string {
	return "post_comment"
}
