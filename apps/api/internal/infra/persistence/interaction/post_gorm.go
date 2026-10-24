package infrainteraction

import (
	"context"
	"errors"
	domainaccount "social/internal/domain/account"
	domainpost "social/internal/domain/post"
	domainpostinteraction "social/internal/domain/postinteraction"
	infrapost "social/internal/infra/persistence/post"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostRepository struct {
	db *gorm.DB
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

type postCommentWithUserModel struct {
	ID             int64
	PostID         int64
	UserID         int64
	UserNickname   string
	UserAvatarURL  string
	Content        string
	Status         int
	IdempotencyKey *string
	CreatedAt      int64
	UpdatedAt      int64
}

func NewPostRepository(db *gorm.DB) *PostRepository {
	return &PostRepository{db: db}
}

func (r *PostRepository) GetActionStat(ctx context.Context, postID int64) (int, int, error) {
	if err := ensurePublishedPost(r.db.WithContext(ctx), postID); err != nil {
		return 0, 0, mapPostInteractionError(err)
	}
	var counts []struct {
		ActionType string
		Count      int
	}
	err := r.db.WithContext(ctx).
		Table("post_action").
		Select("action_type, COUNT(*) AS count").
		Where("post_id = ? AND status = ?", postID, domainpostinteraction.ActionStatusActive).
		Group("action_type").
		Scan(&counts).
		Error
	if err != nil {
		return 0, 0, err
	}
	likeCount := 0
	favoriteCount := 0
	for _, count := range counts {
		if count.ActionType == domainpostinteraction.ActionTypeLike {
			likeCount = count.Count
		} else if count.ActionType == domainpostinteraction.ActionTypeFavorite {
			favoriteCount = count.Count
		}
	}
	return likeCount, favoriteCount, nil
}

func (r *PostRepository) SetAction(ctx context.Context, userID int64, postID int64, actionType string, active bool, idempotencyKey string) (*domainpostinteraction.Action, int, error) {
	var action PostActionModel
	var delta int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensurePublishedPost(tx, postID); err != nil {
			return err
		}
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND post_id = ? AND action_type = ?", userID, postID, actionType).
			Take(&action).
			Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		nextStatus := domainpostinteraction.ActionStatusCanceled
		if active {
			nextStatus = domainpostinteraction.ActionStatusActive
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			action = PostActionModel{
				UserID:         userID,
				PostID:         postID,
				ActionType:     actionType,
				Status:         nextStatus,
				IdempotencyKey: postStringPtr(idempotencyKey),
			}
			if err := tx.Create(&action).Error; err != nil {
				return err
			}
			if active {
				delta = 1
			}
		} else if !(idempotencyKey != "" && postStringValue(action.IdempotencyKey) == idempotencyKey) {
			if action.Status != nextStatus {
				if active {
					delta = 1
				} else {
					delta = -1
				}
			}
			action.Status = nextStatus
			action.IdempotencyKey = postStringPtr(idempotencyKey)
			if err := tx.Save(&action).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, 0, mapPostInteractionError(err)
	}
	return restorePostAction(action), delta, nil
}

func (r *PostRepository) CreateComment(ctx context.Context, comment *domainpostinteraction.Comment) (*domainpostinteraction.Comment, int, int, error) {
	var model PostCommentModel
	var count int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		post, err := lockPublishedPost(tx, comment.PostID)
		if err != nil {
			return err
		}
		model = PostCommentModel{
			PostID:         comment.PostID,
			UserID:         comment.UserID,
			Content:        comment.Content,
			Status:         domainpostinteraction.CommentStatusNormal,
			IdempotencyKey: postStringPtr(comment.IdempotencyKey),
		}
		if err := tx.Create(&model).Error; err != nil {
			if isDuplicateKeyError(err) && comment.IdempotencyKey != "" {
				return domainpostinteraction.ErrDuplicateCommentIdempotencyKey
			}
			return err
		}
		post.CommentCount++
		count = post.CommentCount
		return tx.Model(&infrapost.PostModel{}).Where("id = ?", post.ID).Update("comment_count", post.CommentCount).Error
	})
	if err != nil {
		return nil, 0, 0, mapPostInteractionError(err)
	}
	created, err := r.findPostCommentByID(ctx, model.ID)
	if err != nil {
		return nil, 0, 0, err
	}
	return created, count, 1, nil
}

func (r *PostRepository) FindCommentByUserAndIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (*domainpostinteraction.Comment, int, error) {
	var model postCommentWithUserModel
	result := r.db.WithContext(ctx).
		Table("post_comment AS c").
		Select(postCommentWithUserSelect()).
		Joins("LEFT JOIN account AS a ON a.id = c.user_id").
		Where("c.user_id = ? AND c.idempotency_key = ?", userID, idempotencyKey).
		Limit(1).
		Find(&model)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, 0, domainpostinteraction.ErrCommentNotFound
	}
	count, err := r.postCommentCount(ctx, model.PostID)
	if err != nil {
		return nil, 0, err
	}
	return restorePostComment(model), count, nil
}

func (r *PostRepository) ListComments(ctx context.Context, postID int64, limit int, offset int) ([]*domainpostinteraction.Comment, error) {
	if err := ensurePublishedPost(r.db.WithContext(ctx), postID); err != nil {
		return nil, mapPostInteractionError(err)
	}
	var models []postCommentWithUserModel
	err := r.db.WithContext(ctx).
		Table("post_comment AS c").
		Select(postCommentWithUserSelect()).
		Joins("LEFT JOIN account AS a ON a.id = c.user_id").
		Where("c.post_id = ? AND c.status = ?", postID, domainpostinteraction.CommentStatusNormal).
		Order("c.created_at DESC").
		Order("c.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	comments := make([]*domainpostinteraction.Comment, 0, len(models))
	for _, model := range models {
		comments = append(comments, restorePostComment(model))
	}
	return comments, nil
}

func (r *PostRepository) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*domainpostinteraction.Comment, int, int, error) {
	var model PostCommentModel
	var count int
	var delta int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", commentID).Take(&model).Error; err != nil {
			return err
		}
		post, err := lockPost(tx, model.PostID)
		if err != nil {
			return err
		}
		if model.UserID != userID && post.AuthorID != userID && role != domainaccount.RoleAdmin {
			return domainpostinteraction.ErrCommentPermissionDenied
		}
		if model.Status == domainpostinteraction.CommentStatusDeleted {
			count = post.CommentCount
			return nil
		}
		model.Status = domainpostinteraction.CommentStatusDeleted
		if err := tx.Save(&model).Error; err != nil {
			return err
		}
		post.CommentCount = clampPostCount(post.CommentCount - 1)
		count = post.CommentCount
		delta = -1
		return tx.Model(&infrapost.PostModel{}).Where("id = ?", post.ID).Update("comment_count", post.CommentCount).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, 0, domainpostinteraction.ErrCommentNotFound
		}
		return nil, 0, 0, mapPostInteractionError(err)
	}
	comment, err := r.findPostCommentByID(ctx, model.ID)
	return comment, count, delta, err
}

func (r *PostRepository) GetPostAuthorID(ctx context.Context, postID int64) (int64, error) {
	var post infrapost.PostModel
	err := r.db.WithContext(ctx).Select("id, author_id").Where("id = ? AND status = ?", postID, domainpost.StatusPublished).Take(&post).Error
	if err != nil {
		return 0, mapPostInteractionError(err)
	}
	return post.AuthorID, nil
}

func (r *PostRepository) GetUserProfile(ctx context.Context, userID int64) (*domainpostinteraction.UserProfile, error) {
	var model struct {
		ID        int64
		Nickname  string
		AvatarURL string
	}
	if err := r.db.WithContext(ctx).Table("account").Select("id, nickname, avatar_url").Where("id = ?", userID).Take(&model).Error; err != nil {
		return nil, err
	}
	return &domainpostinteraction.UserProfile{ID: model.ID, Nickname: model.Nickname, AvatarURL: model.AvatarURL}, nil
}

func lockPublishedPost(tx *gorm.DB, postID int64) (*infrapost.PostModel, error) {
	var post infrapost.PostModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", postID, domainpost.StatusPublished).Take(&post).Error
	return &post, err
}

func ensurePublishedPost(db *gorm.DB, postID int64) error {
	var count int64
	if err := db.Model(&infrapost.PostModel{}).Where("id = ? AND status = ?", postID, domainpost.StatusPublished).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func lockPost(tx *gorm.DB, postID int64) (*infrapost.PostModel, error) {
	var post infrapost.PostModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", postID).Take(&post).Error
	return &post, err
}

func (r *PostRepository) findPostCommentByID(ctx context.Context, commentID int64) (*domainpostinteraction.Comment, error) {
	var model postCommentWithUserModel
	err := r.db.WithContext(ctx).
		Table("post_comment AS c").
		Select(postCommentWithUserSelect()).
		Joins("LEFT JOIN account AS a ON a.id = c.user_id").
		Where("c.id = ?", commentID).
		Take(&model).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainpostinteraction.ErrCommentNotFound
	}
	return restorePostComment(model), err
}

func (r *PostRepository) postCommentCount(ctx context.Context, postID int64) (int, error) {
	var post infrapost.PostModel
	if err := r.db.WithContext(ctx).Select("id, comment_count").Where("id = ?", postID).Take(&post).Error; err != nil {
		return 0, mapPostInteractionError(err)
	}
	return post.CommentCount, nil
}

func restorePostAction(model PostActionModel) *domainpostinteraction.Action {
	return &domainpostinteraction.Action{ID: model.ID, UserID: model.UserID, PostID: model.PostID, ActionType: model.ActionType, Status: model.Status, IdempotencyKey: postStringValue(model.IdempotencyKey), CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}

func restorePostComment(model postCommentWithUserModel) *domainpostinteraction.Comment {
	return &domainpostinteraction.Comment{ID: model.ID, PostID: model.PostID, UserID: model.UserID, UserNickname: model.UserNickname, UserAvatarURL: model.UserAvatarURL, Content: model.Content, Status: model.Status, IdempotencyKey: postStringValue(model.IdempotencyKey), CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}

func postCommentWithUserSelect() string {
	return "c.id, c.post_id, c.user_id, a.nickname AS user_nickname, a.avatar_url AS user_avatar_url, c.content, c.status, c.idempotency_key, c.created_at, c.updated_at"
}

func mapPostInteractionError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainpostinteraction.ErrPostNotFound
	}
	return err
}

func postStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func postStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func clampPostCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
