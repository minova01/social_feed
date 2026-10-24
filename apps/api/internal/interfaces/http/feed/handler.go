package interfaceshttpfeed

import (
	"net/http"
	applicationfeed "social/internal/application/feed"
	domainpost "social/internal/domain/post"
	inframetrics "social/internal/infra/metrics"
	interfaceshttpmiddleware "social/internal/interfaces/http/middleware"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	followingPost *applicationfeed.FollowingPostService
	hotPost       *applicationfeed.HotPostService
}

// New 注入 Feed 应用服务。
func New(followingPost *applicationfeed.FollowingPostService, hotPost *applicationfeed.HotPostService) *Handler {
	return &Handler{followingPost: followingPost, hotPost: hotPost}
}

func (h *Handler) ListFollowingPosts(c *gin.Context) {
	viewerID, ok := viewerIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	var req followingPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	start := time.Now()
	result, err := h.followingPost.List(c.Request.Context(), viewerID, req.Limit)
	itemCount := 0
	if result != nil {
		itemCount = len(result.Items)
	}
	inframetrics.ObserveFeed("following", time.Since(start), itemCount, err)
	if err != nil {
		writeFeedError(c, err)
		return
	}
	c.JSON(http.StatusOK, followingPostResponse{Items: postFeedItemResponses(result.Items)})
}

func (h *Handler) ListHotPosts(c *gin.Context) {
	var req hotPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	viewerID, _ := viewerIDFromContext(c)
	start := time.Now()
	result, err := h.hotPost.List(c.Request.Context(), viewerID, req.Limit, req.Offset)
	itemCount := 0
	if result != nil {
		itemCount = len(result.Items)
	}
	inframetrics.ObserveFeed("hot", time.Since(start), itemCount, err)
	if err != nil {
		writeFeedError(c, err)
		return
	}
	c.JSON(http.StatusOK, followingPostResponse{Items: postFeedItemResponses(result.Items)})
}

func postFeedItemResponses(feedItems []*domainpost.FeedItem) []followingPostItemResponse {
	items := make([]followingPostItemResponse, 0, len(feedItems))
	for _, item := range feedItems {
		items = append(items, followingPostItemResponse{
			PostID:          item.PostID,
			AuthorID:        item.AuthorID,
			AuthorNickname:  item.AuthorNickname,
			AuthorAvatarURL: item.AuthorAvatarURL,
			Content:         item.Content,
			ImageURLs:       append([]string(nil), item.ImageURLs...),
			LikeCount:       item.LikeCount,
			CommentCount:    item.CommentCount,
			FavoriteCount:   item.FavoriteCount,
			Liked:           item.Liked,
			Favorited:       item.Favorited,
			PublishedAt:     item.PublishedAt,
		})
	}
	return items
}

func writeFeedError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// viewerIDFromContext 读取可选登录用户 ID，个性化 Feed 策略可以使用。
func viewerIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(interfaceshttpmiddleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}
