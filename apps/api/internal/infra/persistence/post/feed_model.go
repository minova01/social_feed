package infrapost

type FeedStateModel struct {
	UserID         int64 `gorm:"column:user_id;primaryKey"`
	LastSeenAt     int64 `gorm:"column:last_seen_at;not null;default:0"`
	LastSeenPostID int64 `gorm:"column:last_seen_post_id;not null;default:0"`
	CreatedAt      int64 `gorm:"column:created_at;not null;autoCreateTime:milli"`
	UpdatedAt      int64 `gorm:"column:updated_at;not null;autoUpdateTime:milli"`
}

func (FeedStateModel) TableName() string {
	return "user_feed_state"
}
