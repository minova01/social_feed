package interfaceshttppost

import (
	"errors"
	"net/http"
	applicationpost "social/internal/application/post"
	domainpost "social/internal/domain/post"
	interfaceshttpmiddleware "social/internal/interfaces/http/middleware"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultListLimit = 20

type Handler struct {
	service *applicationpost.Service
}

func New(service *applicationpost.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	var header idempotencyHeader
	if err := c.ShouldBindHeader(&header); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid idempotency key"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "post content is required"})
		return
	}
	header.IdempotencyKey = strings.TrimSpace(header.IdempotencyKey)
	var result *applicationpost.CreateResult
	var err error
	if header.IdempotencyKey == "" {
		result, err = h.service.CreatePublished(c.Request.Context(), userID, req.Content, req.ImageURLs)
	} else {
		result, err = h.service.CreatePublishedIdempotently(c.Request.Context(), userID, req.Content, req.ImageURLs, header.IdempotencyKey)
	}
	if err != nil {
		writePostError(c, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, responseFromDomain(result.Post))
}

func (h *Handler) Get(c *gin.Context) {
	var req postIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	post, err := h.service.Get(c.Request.Context(), req.PostID)
	if err != nil {
		writePostError(c, err)
		return
	}
	c.JSON(http.StatusOK, responseFromDomain(post))
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req postIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, req.PostID); err != nil {
		writePostError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListByAuthor(c *gin.Context) {
	var req postListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	h.list(c, req.AuthorID, normalizeLimit(req.Limit), req.Offset)
}

func (h *Handler) ListMine(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req myPostListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	h.list(c, userID, normalizeLimit(req.Limit), req.Offset)
}

func (h *Handler) list(c *gin.Context, authorID int64, limit int, offset int) {
	posts, err := h.service.ListByAuthor(c.Request.Context(), authorID, limit, offset)
	if err != nil {
		writePostError(c, err)
		return
	}
	items := make([]postResponse, 0, len(posts))
	for _, post := range posts {
		items = append(items, responseFromDomain(post))
	}
	c.JSON(http.StatusOK, postListResponse{Items: items, Limit: limit, Offset: offset})
}

func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(interfaceshttpmiddleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

func normalizeLimit(limit int) int {
	if limit == 0 {
		return defaultListLimit
	}
	return limit
}

func responseFromDomain(post *domainpost.Post) postResponse {
	return postResponse{
		ID:            post.ID,
		AuthorID:      post.AuthorID,
		Content:       post.Content,
		ImageURLs:     append([]string(nil), post.ImageURLs...),
		Status:        post.Status,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		PublishedAt:   post.PublishedAt,
		CreatedAt:     post.CreatedAt,
		UpdatedAt:     post.UpdatedAt,
	}
}

func writePostError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainpost.ErrPostNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
	case errors.Is(err, domainpost.ErrPostPermissionDenied):
		c.JSON(http.StatusForbidden, gin.H{"error": "post permission denied"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
