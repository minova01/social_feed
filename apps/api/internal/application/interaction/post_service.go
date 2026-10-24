package applicationinteraction

import (
	"context"
	"errors"
	"fmt"
	domainmessage "social/internal/domain/message"
	domainpost "social/internal/domain/post"
	domainpostinteraction "social/internal/domain/postinteraction"
	"time"
)

var ErrLoadPostInteractionFailed = errors.New("failed to load post interaction")
var ErrSavePostInteractionFailed = errors.New("failed to save post interaction")
var ErrUpdatePostInteractionFailed = errors.New("failed to update post interaction")

type PostHotScoreRecorder interface {
	AddPostHotScore(ctx context.Context, postID int64, scoreDelta int, at time.Time) error
}

type MessageWriter interface {
	CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error)
}

type ActorMessageWriter interface {
	CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error)
}

type PostService struct {
	repo             domainpostinteraction.Repository
	hotScoreRecorder PostHotScoreRecorder
	statStore        PostStatStore
	messageWriter    MessageWriter
}

type PostStatStore interface {
	EnsurePostStat(ctx context.Context, postID int64, loader func(context.Context, int64) (*domainpost.Stat, error)) (*domainpost.Stat, error)
	ApplyPostActionDelta(ctx context.Context, postID int64, actionType string, delta int) (*domainpost.Stat, error)
}

type PostOption func(*PostService)

type PostActionResult struct {
	PostID        int64
	ActionType    string
	Active        bool
	LikeCount     int
	FavoriteCount int
}

type PostCreateCommentResult struct {
	Comment      *domainpostinteraction.Comment
	CommentCount int
}

type PostDeleteCommentResult struct {
	CommentID    int64
	Status       int
	CommentCount int
}

func NewPostService(repo domainpostinteraction.Repository, options ...PostOption) *PostService {
	service := &PostService{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithPostHotScoreRecorder(recorder PostHotScoreRecorder) PostOption {
	return func(service *PostService) {
		service.hotScoreRecorder = recorder
	}
}

func WithPostStatStore(store PostStatStore) PostOption {
	return func(service *PostService) {
		service.statStore = store
	}
}

func WithPostMessageWriter(writer MessageWriter) PostOption {
	return func(service *PostService) {
		service.messageWriter = writer
	}
}

func (s *PostService) Like(ctx context.Context, userID int64, postID int64, idempotencyKey string) (*PostActionResult, error) {
	return s.setAction(ctx, userID, postID, domainpostinteraction.ActionTypeLike, true, idempotencyKey)
}

func (s *PostService) Unlike(ctx context.Context, userID int64, postID int64, idempotencyKey string) (*PostActionResult, error) {
	return s.setAction(ctx, userID, postID, domainpostinteraction.ActionTypeLike, false, idempotencyKey)
}

func (s *PostService) Favorite(ctx context.Context, userID int64, postID int64, idempotencyKey string) (*PostActionResult, error) {
	return s.setAction(ctx, userID, postID, domainpostinteraction.ActionTypeFavorite, true, idempotencyKey)
}

func (s *PostService) Unfavorite(ctx context.Context, userID int64, postID int64, idempotencyKey string) (*PostActionResult, error) {
	return s.setAction(ctx, userID, postID, domainpostinteraction.ActionTypeFavorite, false, idempotencyKey)
}

func (s *PostService) setAction(ctx context.Context, userID int64, postID int64, actionType string, active bool, idempotencyKey string) (*PostActionResult, error) {
	statReady := false
	if s.statStore != nil {
		_, statErr := s.statStore.EnsurePostStat(ctx, postID, func(ctx context.Context, postID int64) (*domainpost.Stat, error) {
			likeCount, favoriteCount, err := s.repo.GetActionStat(ctx, postID)
			if err != nil {
				return nil, err
			}
			return &domainpost.Stat{PostID: postID, LikeCount: likeCount, FavoriteCount: favoriteCount}, nil
		})
		statReady = statErr == nil
	}
	action, delta, err := s.repo.SetAction(ctx, userID, postID, actionType, active, idempotencyKey)
	if err != nil {
		if errors.Is(err, domainpostinteraction.ErrPostNotFound) {
			return nil, err
		}
		return nil, ErrUpdatePostInteractionFailed
	}
	result := &PostActionResult{PostID: action.PostID, ActionType: action.ActionType, Active: action.Active()}
	weight := 4
	if action.ActionType == domainpostinteraction.ActionTypeLike {
		weight = 3
	}
	if s.statStore != nil && statReady {
		if stat, statErr := s.statStore.ApplyPostActionDelta(ctx, action.PostID, action.ActionType, delta); statErr == nil {
			result.LikeCount = stat.LikeCount
			result.FavoriteCount = stat.FavoriteCount
		}
	}
	s.recordHotScore(ctx, action.PostID, delta*weight)
	if action.ActionType == domainpostinteraction.ActionTypeLike && action.Active() && delta > 0 {
		s.notifyLike(ctx, action)
	}
	return result, nil
}

func (s *PostService) CreateComment(ctx context.Context, userID int64, postID int64, content string) (*PostCreateCommentResult, error) {
	return s.createComment(ctx, &domainpostinteraction.Comment{PostID: postID, UserID: userID, Content: content, Status: domainpostinteraction.CommentStatusNormal})
}

func (s *PostService) CreateCommentIdempotently(ctx context.Context, userID int64, postID int64, content string, idempotencyKey string) (*PostCreateCommentResult, error) {
	existing, count, err := s.repo.FindCommentByUserAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err == nil {
		return &PostCreateCommentResult{Comment: existing, CommentCount: count}, nil
	}
	if !errors.Is(err, domainpostinteraction.ErrCommentNotFound) {
		return nil, ErrLoadPostInteractionFailed
	}
	return s.createComment(ctx, &domainpostinteraction.Comment{PostID: postID, UserID: userID, Content: content, Status: domainpostinteraction.CommentStatusNormal, IdempotencyKey: idempotencyKey})
}

func (s *PostService) createComment(ctx context.Context, comment *domainpostinteraction.Comment) (*PostCreateCommentResult, error) {
	created, count, delta, err := s.repo.CreateComment(ctx, comment)
	if errors.Is(err, domainpostinteraction.ErrDuplicateCommentIdempotencyKey) {
		existing, existingCount, loadErr := s.repo.FindCommentByUserAndIdempotencyKey(ctx, comment.UserID, comment.IdempotencyKey)
		if loadErr == nil {
			return &PostCreateCommentResult{Comment: existing, CommentCount: existingCount}, nil
		}
		return nil, ErrLoadPostInteractionFailed
	}
	if err != nil {
		if errors.Is(err, domainpostinteraction.ErrPostNotFound) {
			return nil, err
		}
		return nil, ErrSavePostInteractionFailed
	}
	s.recordHotScore(ctx, created.PostID, delta*5)
	if delta > 0 {
		s.notifyComment(ctx, created)
	}
	return &PostCreateCommentResult{Comment: created, CommentCount: count}, nil
}

func (s *PostService) ListComments(ctx context.Context, postID int64, limit int, offset int) ([]*domainpostinteraction.Comment, error) {
	comments, err := s.repo.ListComments(ctx, postID, limit, offset)
	if err != nil {
		if errors.Is(err, domainpostinteraction.ErrPostNotFound) {
			return nil, err
		}
		return nil, ErrLoadPostInteractionFailed
	}
	return comments, nil
}

func (s *PostService) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*PostDeleteCommentResult, error) {
	comment, count, delta, err := s.repo.DeleteComment(ctx, commentID, userID, role)
	if err != nil {
		if errors.Is(err, domainpostinteraction.ErrCommentNotFound) || errors.Is(err, domainpostinteraction.ErrCommentPermissionDenied) || errors.Is(err, domainpostinteraction.ErrPostNotFound) {
			return nil, err
		}
		return nil, ErrUpdatePostInteractionFailed
	}
	s.recordHotScore(ctx, comment.PostID, delta*5)
	return &PostDeleteCommentResult{CommentID: comment.ID, Status: comment.Status, CommentCount: count}, nil
}

func (s *PostService) recordHotScore(ctx context.Context, postID int64, delta int) {
	if s.hotScoreRecorder != nil && delta != 0 {
		_ = s.hotScoreRecorder.AddPostHotScore(ctx, postID, delta, time.Now())
	}
}

func (s *PostService) notifyLike(ctx context.Context, action *domainpostinteraction.Action) {
	authorID, err := s.repo.GetPostAuthorID(ctx, action.PostID)
	if s.messageWriter == nil || err != nil || authorID == action.UserID {
		return
	}
	eventID := fmt.Sprintf("post:like:%d:%d", action.PostID, action.UserID)
	s.createMessage(ctx, authorID, domainmessage.TypeLike, "收到点赞", "点赞了你的帖子", eventID, action.UserID)
}

func (s *PostService) notifyComment(ctx context.Context, comment *domainpostinteraction.Comment) {
	authorID, err := s.repo.GetPostAuthorID(ctx, comment.PostID)
	if s.messageWriter == nil || err != nil || authorID == comment.UserID {
		return
	}
	eventID := fmt.Sprintf("post:comment:%d", comment.ID)
	s.createMessage(ctx, authorID, domainmessage.TypeComment, "收到评论", comment.Content, eventID, comment.UserID)
}

func (s *PostService) createMessage(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, actorID int64) {
	profile, _ := s.repo.GetUserProfile(ctx, actorID)
	if writer, ok := s.messageWriter.(ActorMessageWriter); ok {
		nickname := ""
		avatarURL := ""
		if profile != nil {
			nickname = profile.Nickname
			avatarURL = profile.AvatarURL
		}
		_, _ = writer.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, eventID, actorID, nickname, avatarURL)
		return
	}
	actorName := fmt.Sprintf("用户 %d", actorID)
	if profile != nil && profile.Nickname != "" {
		actorName = profile.Nickname
	}
	_, _ = s.messageWriter.CreateFromEvent(ctx, userID, messageType, title, fmt.Sprintf("%s %s", actorName, content), eventID, eventID)
}
