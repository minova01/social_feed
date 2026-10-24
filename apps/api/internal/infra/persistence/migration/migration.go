package migration

import (
	"errors"
	infraaccount "social/internal/infra/persistence/account"
	infrainteraction "social/internal/infra/persistence/interaction"
	inframessage "social/internal/infra/persistence/message"
	infrapost "social/internal/infra/persistence/post"
	infrarelation "social/internal/infra/persistence/relation"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	return autoMigrateModels(db)
}

func autoMigrateModels(db *gorm.DB) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = db.AutoMigrate(
			&infraaccount.UserModel{},
			&infrainteraction.PostActionModel{},
			&infrainteraction.PostCommentModel{},
			&inframessage.MessageModel{},
			&infrapost.PostModel{},
			&infrapost.FeedStateModel{},
			&infrarelation.FollowModel{},
			&infrarelation.RelationStatModel{},
		)
		if err == nil {
			return nil
		}
		if !isConcurrentMigrationError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	return err
}

func isConcurrentMigrationError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	switch mysqlErr.Number {
	case 1060, 1061:
		return true
	default:
		return false
	}
}
