package infrapost

import (
	"context"
	"errors"
	domainpost "social/internal/domain/post"
	domainpostinteraction "social/internal/domain/postinteraction"
	domainrelation "social/internal/domain/relation"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Save(ctx context.Context, post *domainpost.Post) error {
	model := modelFromDomain(post)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isDuplicateKeyError(err) {
			return domainpost.ErrDuplicateIdempotencyKey
		}
		return err
	}
	post.ID = model.ID
	post.CreatedAt = model.CreatedAt
	post.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*domainpost.Post, error) {
	var model PostModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, domainpost.StatusPublished).
		Take(&model).
		Error
	if err != nil {
		return nil, mapNotFound(err)
	}
	return restore(model), nil
}

func (r *Repository) FindByIDAnyStatus(ctx context.Context, id int64) (*domainpost.Post, error) {
	var model PostModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&model).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return restore(model), nil
}

func (r *Repository) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainpost.Post, error) {
	var model PostModel
	if err := r.db.WithContext(ctx).
		Where("author_id = ? AND idempotency_key = ?", authorID, key).
		Take(&model).
		Error; err != nil {
		return nil, mapNotFound(err)
	}
	return restore(model), nil
}

func (r *Repository) ListByAuthor(ctx context.Context, authorID int64, limit int, offset int) ([]*domainpost.Post, error) {
	var models []PostModel
	if err := r.db.WithContext(ctx).
		Where("author_id = ? AND status = ?", authorID, domainpost.StatusPublished).
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Offset(offset).
		Find(&models).
		Error; err != nil {
		return nil, err
	}
	posts := make([]*domainpost.Post, 0, len(models))
	for _, model := range models {
		posts = append(posts, restore(model))
	}
	return posts, nil
}

func (r *Repository) CountFollowers(ctx context.Context, authorID int64) (int, error) {
	var count int
	err := r.db.WithContext(ctx).
		Table("user_relation_stat").
		Select("follower_count").
		Where("user_id = ?", authorID).
		Take(&count).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return count, err
}

func (r *Repository) ListFollowerIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error) {
	var followerIDs []int64
	query := r.db.WithContext(ctx).
		Table("user_follow").
		Select("user_id").
		Where("target_user_id = ? AND status = ?", authorID, domainrelation.FollowStatusActive)
	if cursor > 0 {
		query = query.Where("user_id > ?", cursor)
	}
	err := query.Order("user_id ASC").Limit(limit).Scan(&followerIDs).Error
	return followerIDs, err
}

func (r *Repository) ListRecentTimelineItems(ctx context.Context, authorID int64, limit int) ([]*domainpost.TimelineItem, error) {
	var items []*domainpost.TimelineItem
	err := r.db.WithContext(ctx).
		Table("post").
		Select("id AS post_id, author_id, published_at").
		Where("author_id = ? AND status = ?", authorID, domainpost.StatusPublished).
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Scan(&items).
		Error
	return items, err
}

func (r *Repository) GetFeedState(ctx context.Context, userID int64) (*domainpost.FeedState, error) {
	var model FeedStateModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domainpost.FeedState{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &domainpost.FeedState{
		UserID:         model.UserID,
		LastSeenAt:     model.LastSeenAt,
		LastSeenPostID: model.LastSeenPostID,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
	}, nil
}

func (r *Repository) SaveFeedState(ctx context.Context, state *domainpost.FeedState) error {
	model := FeedStateModel{
		UserID:         state.UserID,
		LastSeenAt:     state.LastSeenAt,
		LastSeenPostID: state.LastSeenPostID,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_seen_at",
			"last_seen_post_id",
			"updated_at",
		}),
	}).Create(&model).Error
}

func (r *Repository) ListFollowingBigCreatorIDs(ctx context.Context, viewerID int64) ([]int64, error) {
	var authorIDs []int64
	err := r.db.WithContext(ctx).
		Table("user_follow AS f").
		Select("f.target_user_id").
		Joins("LEFT JOIN user_relation_stat AS rs ON rs.user_id = f.target_user_id").
		Where("f.user_id = ? AND f.status = ? AND COALESCE(rs.follower_count, 0) >= ?", viewerID, domainrelation.FollowStatusActive, domainpost.BigCreatorFollowerThreshold).
		Order("f.target_user_id ASC").
		Scan(&authorIDs).
		Error
	return authorIDs, err
}

func (r *Repository) ListFollowingPostWindow(ctx context.Context, viewerID int64, bigCreator bool, after domainpost.TimelineCursor, cutoffAt int64, limit int) (*domainpost.TimelineBatch, error) {
	var items []*domainpost.TimelineItem
	query := r.followingPostQuery(ctx, viewerID, bigCreator).
		Where("(p.published_at > ? OR (p.published_at = ? AND p.id > ?))", after.PublishedAt, after.PublishedAt, after.PostID).
		Where("p.published_at <= ?", cutoffAt)
	err := query.
		Order("p.published_at ASC").
		Order("p.id ASC").
		Limit(limit).
		Scan(&items).
		Error
	return &domainpost.TimelineBatch{Items: items, Capped: len(items) == limit}, err
}

func (r *Repository) ListFollowingRecentPosts(ctx context.Context, viewerID int64, bigCreator bool, cutoffAt int64, limit int) ([]*domainpost.TimelineItem, error) {
	var items []*domainpost.TimelineItem
	err := r.followingPostQuery(ctx, viewerID, bigCreator).
		Where("p.published_at <= ?", cutoffAt).
		Order("p.published_at DESC").
		Order("p.id DESC").
		Limit(limit).
		Scan(&items).
		Error
	return items, err
}

func (r *Repository) followingPostQuery(ctx context.Context, viewerID int64, bigCreator bool) *gorm.DB {
	operator := "<"
	if bigCreator {
		operator = ">="
	}
	return r.db.WithContext(ctx).
		Table("post AS p").
		Select("p.id AS post_id, p.author_id, p.published_at").
		Joins("JOIN user_follow AS f ON f.target_user_id = p.author_id").
		Joins("LEFT JOIN user_relation_stat AS rs ON rs.user_id = p.author_id").
		Where("f.user_id = ? AND f.status = ?", viewerID, domainrelation.FollowStatusActive).
		Where("p.status = ?", domainpost.StatusPublished).
		Where("COALESCE(rs.follower_count, 0) "+operator+" ?", domainpost.BigCreatorFollowerThreshold)
}

func (r *Repository) BatchGetFollowingFeedItems(ctx context.Context, viewerID int64, postIDs []int64) (map[int64]*domainpost.FeedItem, error) {
	items := map[int64]*domainpost.FeedItem{}
	if len(postIDs) == 0 {
		return items, nil
	}
	var models []struct {
		PostID          int64
		AuthorID        int64
		AuthorNickname  string
		AuthorAvatarURL string
		Content         string
		ImageURLs       []string `gorm:"serializer:json"`
		LikeCount       int
		CommentCount    int
		FavoriteCount   int
		PublishedAt     int64
	}
	err := r.db.WithContext(ctx).
		Table("post AS p").
		Select("p.id AS post_id, p.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, p.content, p.image_urls, p.like_count, p.comment_count, p.favorite_count, p.published_at").
		Joins("JOIN user_follow AS f ON f.target_user_id = p.author_id AND f.user_id = ? AND f.status = ?", viewerID, domainrelation.FollowStatusActive).
		Joins("LEFT JOIN account AS a ON a.id = p.author_id").
		Where("p.id IN ? AND p.status = ?", postIDs, domainpost.StatusPublished).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		items[model.PostID] = &domainpost.FeedItem{
			PostID:          model.PostID,
			AuthorID:        model.AuthorID,
			AuthorNickname:  model.AuthorNickname,
			AuthorAvatarURL: model.AuthorAvatarURL,
			Content:         model.Content,
			ImageURLs:       append([]string(nil), model.ImageURLs...),
			LikeCount:       model.LikeCount,
			CommentCount:    model.CommentCount,
			FavoriteCount:   model.FavoriteCount,
			PublishedAt:     model.PublishedAt,
		}
	}
	if err := r.applyViewerPostActions(ctx, viewerID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListHotPostCandidates(ctx context.Context, limit int, offset int) ([]*domainpost.HotItem, error) {
	var items []*domainpost.HotItem
	err := r.db.WithContext(ctx).
		Table("post").
		Select("id AS post_id, like_count * 3 + comment_count * 5 + favorite_count * 4 AS hot_score").
		Where("status = ?", domainpost.StatusPublished).
		Order("hot_score DESC").
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).
		Error
	return items, err
}

func (r *Repository) BatchGetPublicFeedItems(ctx context.Context, viewerID int64, postIDs []int64) (map[int64]*domainpost.FeedItem, error) {
	items := map[int64]*domainpost.FeedItem{}
	if len(postIDs) == 0 {
		return items, nil
	}
	var models []struct {
		PostID          int64
		AuthorID        int64
		AuthorNickname  string
		AuthorAvatarURL string
		Content         string
		ImageURLs       []string `gorm:"serializer:json"`
		LikeCount       int
		CommentCount    int
		FavoriteCount   int
		PublishedAt     int64
	}
	err := r.db.WithContext(ctx).
		Table("post AS p").
		Select("p.id AS post_id, p.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, p.content, p.image_urls, p.like_count, p.comment_count, p.favorite_count, p.published_at").
		Joins("LEFT JOIN account AS a ON a.id = p.author_id").
		Where("p.id IN ? AND p.status = ?", postIDs, domainpost.StatusPublished).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		items[model.PostID] = &domainpost.FeedItem{
			PostID:          model.PostID,
			AuthorID:        model.AuthorID,
			AuthorNickname:  model.AuthorNickname,
			AuthorAvatarURL: model.AuthorAvatarURL,
			Content:         model.Content,
			ImageURLs:       append([]string(nil), model.ImageURLs...),
			LikeCount:       model.LikeCount,
			CommentCount:    model.CommentCount,
			FavoriteCount:   model.FavoriteCount,
			PublishedAt:     model.PublishedAt,
		}
	}
	if err := r.applyViewerPostActions(ctx, viewerID, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) applyViewerPostActions(ctx context.Context, viewerID int64, items map[int64]*domainpost.FeedItem) error {
	if viewerID <= 0 || len(items) == 0 {
		return nil
	}
	postIDs := make([]int64, 0, len(items))
	for postID := range items {
		postIDs = append(postIDs, postID)
	}
	var actions []struct {
		PostID     int64
		ActionType string
	}
	if err := r.db.WithContext(ctx).
		Table("post_action").
		Select("post_id, action_type").
		Where("user_id = ? AND post_id IN ? AND status = ?", viewerID, postIDs, domainpostinteraction.ActionStatusActive).
		Scan(&actions).
		Error; err != nil {
		return err
	}
	for _, action := range actions {
		item := items[action.PostID]
		if item == nil {
			continue
		}
		if action.ActionType == domainpostinteraction.ActionTypeLike {
			item.Liked = true
		} else if action.ActionType == domainpostinteraction.ActionTypeFavorite {
			item.Favorited = true
		}
	}
	return nil
}

func (r *Repository) UpdateStatus(ctx context.Context, post *domainpost.Post) error {
	result := r.db.WithContext(ctx).
		Model(&PostModel{}).
		Where("id = ?", post.ID).
		Updates(map[string]any{"status": post.Status, "updated_at": post.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainpost.ErrPostNotFound
	}
	return nil
}

func modelFromDomain(post *domainpost.Post) PostModel {
	return PostModel{
		AuthorID:       post.AuthorID,
		Content:        post.Content,
		ImageURLs:      append([]string(nil), post.ImageURLs...),
		Status:         post.Status,
		LikeCount:      post.LikeCount,
		CommentCount:   post.CommentCount,
		FavoriteCount:  post.FavoriteCount,
		PublishedAt:    post.PublishedAt,
		IdempotencyKey: stringPtr(post.IdempotencyKey),
	}
}

func restore(model PostModel) *domainpost.Post {
	return domainpost.Restore(
		model.ID,
		model.AuthorID,
		model.Content,
		model.ImageURLs,
		model.Status,
		model.LikeCount,
		model.CommentCount,
		model.FavoriteCount,
		model.PublishedAt,
		model.CreatedAt,
		model.UpdatedAt,
		stringValue(model.IdempotencyKey),
	)
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainpost.ErrPostNotFound
	}
	return err
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
