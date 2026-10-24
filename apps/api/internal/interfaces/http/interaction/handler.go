package interfaceshttpinteraction

import (
	"errors"
	"net/http"
	applicationinteraction "social/internal/application/interaction"
	domainpostinteraction "social/internal/domain/postinteraction"
	interfaceshttpmiddleware "social/internal/interfaces/http/middleware"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	postService *applicationinteraction.PostService
}

func New(postService *applicationinteraction.PostService) *Handler {
	return &Handler{postService: postService}
}
func (h *Handler) LikePost(c *gin.Context) {
	h.setPostAction(c, domainpostinteraction.ActionTypeLike, true)
}
func (h *Handler) UnlikePost(c *gin.Context) {
	h.setPostAction(c, domainpostinteraction.ActionTypeLike, false)
}
func (h *Handler) FavoritePost(c *gin.Context) {
	h.setPostAction(c, domainpostinteraction.ActionTypeFavorite, true)
}
func (h *Handler) UnfavoritePost(c *gin.Context) {
	h.setPostAction(c, domainpostinteraction.ActionTypeFavorite, false)
}
func (h *Handler) setPostAction(c *gin.Context, actionType string, active bool) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req postActionRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	header, ok := bindPostIdempotencyHeader(c)
	if !ok {
		return
	}
	var result *applicationinteraction.PostActionResult
	var err error
	switch {
	case actionType == domainpostinteraction.ActionTypeLike && active:
		result, err = h.postService.Like(c.Request.Context(), userID, req.PostID, header.IdempotencyKey)
	case actionType == domainpostinteraction.ActionTypeLike:
		result, err = h.postService.Unlike(c.Request.Context(), userID, req.PostID, header.IdempotencyKey)
	case active:
		result, err = h.postService.Favorite(c.Request.Context(), userID, req.PostID, header.IdempotencyKey)
	default:
		result, err = h.postService.Unfavorite(c.Request.Context(), userID, req.PostID, header.IdempotencyKey)
	}
	if err != nil {
		writePostInteractionError(c, err)
		return
	}
	c.JSON(http.StatusOK, postActionResponse{PostID: result.PostID, ActionType: result.ActionType, Active: result.Active, LikeCount: result.LikeCount, FavoriteCount: result.FavoriteCount})
}
func (h *Handler) CreatePostComment(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req postCommentCreateRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment content is required"})
		return
	}
	header, ok := bindPostIdempotencyHeader(c)
	if !ok {
		return
	}
	var result *applicationinteraction.PostCreateCommentResult
	var err error
	if header.IdempotencyKey == "" {
		result, err = h.postService.CreateComment(c.Request.Context(), userID, req.PostID, req.Content)
	} else {
		result, err = h.postService.CreateCommentIdempotently(c.Request.Context(), userID, req.PostID, req.Content, header.IdempotencyKey)
	}
	if err != nil {
		writePostInteractionError(c, err)
		return
	}
	response := postCommentResponseFromDomain(result.Comment)
	response.CommentCount = result.CommentCount
	c.JSON(http.StatusCreated, response)
}
func (h *Handler) ListPostComments(c *gin.Context) {
	var req postCommentListRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	comments, err := h.postService.ListComments(c.Request.Context(), req.PostID, req.Limit, req.Offset)
	if err != nil {
		writePostInteractionError(c, err)
		return
	}
	items := make([]postCommentResponse, 0, len(comments))
	for _, comment := range comments {
		items = append(items, postCommentResponseFromDomain(comment))
	}
	c.JSON(http.StatusOK, postCommentListResponse{Items: items, Limit: req.Limit, Offset: req.Offset})
}
func (h *Handler) DeletePostComment(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req postCommentDeleteRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	result, err := h.postService.DeleteComment(c.Request.Context(), req.CommentID, userID, roleFromContext(c))
	if err != nil {
		writePostInteractionError(c, err)
		return
	}
	c.JSON(http.StatusOK, deleteCommentResponse{CommentID: result.CommentID, Status: result.Status, CommentCount: result.CommentCount})
}
func bindPostIdempotencyHeader(c *gin.Context) (postInteractionIdempotencyHeader, bool) {
	var header postInteractionIdempotencyHeader
	if c.ShouldBindHeader(&header) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid idempotency key"})
		return header, false
	}
	header.IdempotencyKey = strings.TrimSpace(header.IdempotencyKey)
	return header, true
}
func postCommentResponseFromDomain(comment *domainpostinteraction.Comment) postCommentResponse {
	return postCommentResponse{ID: comment.ID, PostID: comment.PostID, UserID: comment.UserID, UserNickname: comment.UserNickname, UserAvatarURL: comment.UserAvatarURL, Content: comment.Content, CreatedAt: comment.CreatedAt}
}
func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(interfaceshttpmiddleware.ContextUserIDKey)
	userID, ok := value.(int64)
	return userID, exists && ok && userID > 0
}
func roleFromContext(c *gin.Context) string {
	value, _ := c.Get(interfaceshttpmiddleware.ContextRoleKey)
	role, _ := value.(string)
	return role
}
func writePostInteractionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainpostinteraction.ErrPostNotFound), errors.Is(err, domainpostinteraction.ErrCommentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
	case errors.Is(err, domainpostinteraction.ErrCommentPermissionDenied):
		c.JSON(http.StatusForbidden, gin.H{"error": "comment permission denied"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
