package interfaceshttprouter

import (
	"context"
	"database/sql"
	applicationaccount "social/internal/application/account"
	applicationfeed "social/internal/application/feed"
	applicationinteraction "social/internal/application/interaction"
	applicationmessage "social/internal/application/message"
	applicationpost "social/internal/application/post"
	applicationrelation "social/internal/application/relation"
	domainpost "social/internal/domain/post"
	infracache "social/internal/infra/cache"
	infraconfig "social/internal/infra/config"
	infrajwt "social/internal/infra/jwt"
	inframq "social/internal/infra/mq"
	infraaccount "social/internal/infra/persistence/account"
	infrainteraction "social/internal/infra/persistence/interaction"
	inframessage "social/internal/infra/persistence/message"
	migration "social/internal/infra/persistence/migration"
	infrapost "social/internal/infra/persistence/post"
	infrarelation "social/internal/infra/persistence/relation"
	infrasearch "social/internal/infra/search"
	interfaceshttpaccount "social/internal/interfaces/http/account"
	interfaceshttpfeed "social/internal/interfaces/http/feed"
	interfaceshttpinteraction "social/internal/interfaces/http/interaction"
	interfaceshttpmessage "social/internal/interfaces/http/message"
	interfaceshttpmiddleware "social/internal/interfaces/http/middleware"
	interfaceshttppost "social/internal/interfaces/http/post"
	interfaceshttprelation "social/internal/interfaces/http/relation"
	interfaceshttpupload "social/internal/interfaces/http/upload"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Register(g *gin.Engine, cfg *infraconfig.Config, db *sql.DB) error {
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		return err
	}
	if err := migration.AutoMigrate(gormDB); err != nil {
		return err
	}
	jwtManager, err := infrajwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	if err != nil {
		return err
	}

	accountHandler := interfaceshttpaccount.New(applicationaccount.New(infraaccount.New(gormDB), jwtManager))
	postRepo := infrapost.New(gormDB)
	postOptions := []applicationpost.Option{}
	searchClient, err := infrasearch.New(cfg.Search)
	if err != nil {
		return err
	}
	postOptions = append(postOptions, applicationpost.WithSearchIndex(searchClient))
	var feedCache *infracache.FeedCache
	if cfg.Redis.Addr != "" {
		feedCache = infracache.NewFeedCache(infracache.NewRedisClient(cfg.Redis))
		postOptions = append(postOptions, applicationpost.WithStatReader(feedCache))
	}
	kafkaClient, err := inframq.NewKafka(cfg.Kafka)
	if err != nil {
		return err
	}
	postOptions = append(postOptions, applicationpost.WithPostEventPublisher(kafkaClient))
	postHandler := interfaceshttppost.New(applicationpost.New(postRepo, postOptions...))
	followingPostService := applicationfeed.NewFollowingPostService(postRepo, feedCache)
	hotPostService := applicationfeed.NewHotPostService(postRepo, feedCache)
	feedHandler := interfaceshttpfeed.New(followingPostService, hotPostService)

	messageService := applicationmessage.New(inframessage.New(gormDB))
	messageHandler := interfaceshttpmessage.New(messageService)
	messageWriter := NewMessageWriter(messageService)
	postInteractionOptions := []applicationinteraction.PostOption{applicationinteraction.WithPostMessageWriter(messageWriter)}
	if feedCache != nil {
		postInteractionOptions = append(postInteractionOptions, applicationinteraction.WithPostHotScoreRecorder(feedCache), applicationinteraction.WithPostStatStore(feedCache))
	}
	postInteractionService := applicationinteraction.NewPostService(infrainteraction.NewPostRepository(gormDB), postInteractionOptions...)
	interactionHandler := interfaceshttpinteraction.New(postInteractionService)

	relationOptions := []applicationrelation.Option{applicationrelation.WithMessageWriter(messageWriter)}
	if feedCache != nil {
		relationOptions = append(relationOptions, applicationrelation.WithFollowFeedBackfiller(NewFollowFeedBackfiller(postRepo, feedCache)))
	}
	relationHandler := interfaceshttprelation.New(applicationrelation.New(infrarelation.New(gormDB), relationOptions...))
	uploadHandler := interfaceshttpupload.New(cfg.Upload.Root)

	g.GET("/health", HealthCheck)
	g.GET("/metrics", gin.WrapH(promhttp.Handler()))
	g.Static("/uploads", cfg.Upload.Root)
	auth := interfaceshttpmiddleware.NewJWTAuth(jwtManager)
	optionalAuth := interfaceshttpmiddleware.NewOptionalJWTAuth(jwtManager)
	api := g.Group("/api")

	api.POST("/sessions/login", accountHandler.Login)
	api.POST("/sessions/logout", auth, accountHandler.Logout)
	api.POST("/users/register", accountHandler.Register)
	api.POST("/users/me/detail", auth, accountHandler.Me)
	api.POST("/users/me/update", auth, accountHandler.UpdateMe)
	api.POST("/users/detail", accountHandler.Get)
	api.POST("/relations/follow", auth, relationHandler.Follow)
	api.POST("/relations/unfollow", auth, relationHandler.Unfollow)
	api.POST("/relations/following/list", auth, relationHandler.ListFollowing)
	api.POST("/relations/followers/list", auth, relationHandler.ListFollowers)

	posts := api.Group("/posts")
	posts.POST("/create", auth, postHandler.Create)
	posts.POST("/detail", postHandler.Get)
	posts.POST("/delete", auth, postHandler.Delete)
	posts.POST("/list", postHandler.ListByAuthor)
	posts.POST("/list-mine", auth, postHandler.ListMine)
	posts.POST("/search", optionalAuth, postHandler.Search)
	posts.POST("/like", auth, interactionHandler.LikePost)
	posts.POST("/unlike", auth, interactionHandler.UnlikePost)
	posts.POST("/favorite", auth, interactionHandler.FavoritePost)
	posts.POST("/unfavorite", auth, interactionHandler.UnfavoritePost)
	posts.POST("/comments/create", auth, interactionHandler.CreatePostComment)
	posts.POST("/comments/list", interactionHandler.ListPostComments)
	posts.POST("/comments/delete", auth, interactionHandler.DeletePostComment)
	api.POST("/feed/following", auth, feedHandler.ListFollowingPosts)
	api.POST("/feed/hot", optionalAuth, feedHandler.ListHotPosts)
	api.POST("/messages/list", auth, messageHandler.List)
	api.POST("/messages/read", auth, messageHandler.MarkRead)
	api.POST("/messages/unread-count", auth, messageHandler.CountUnread)
	api.POST("/uploads/create", auth, uploadHandler.Create)
	internal := g.Group("/internal", interfaceshttpmiddleware.NewInternalTokenAuth(cfg.Internal.Token))
	internal.POST("/messages/create", messageHandler.Create)
	return nil
}

type MessageWriter struct{ service *applicationmessage.Service }

func NewMessageWriter(service *applicationmessage.Service) *MessageWriter {
	return &MessageWriter{service: service}
}
func (w *MessageWriter) CreateFromEvent(ctx context.Context, userID int64, messageType, title, content, eventID, idempotencyKey string) (any, error) {
	return w.service.CreateFromEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey)
}
func (w *MessageWriter) CreateFromActorEvent(ctx context.Context, userID int64, messageType, title, content, eventID, idempotencyKey string, actorID int64, actorNickname, actorAvatarURL string) (any, error) {
	return w.service.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey, actorID, actorNickname, actorAvatarURL)
}

type FollowFeedBackfiller struct {
	postRepo interface {
		CountFollowers(context.Context, int64) (int, error)
		ListRecentTimelineItems(context.Context, int64, int) ([]*domainpost.TimelineItem, error)
	}
	feedCache interface {
		AddPostInboxItems(context.Context, []int64, *domainpost.TimelineItem, int64) error
	}
}

func NewFollowFeedBackfiller(postRepo interface {
	CountFollowers(context.Context, int64) (int, error)
	ListRecentTimelineItems(context.Context, int64, int) ([]*domainpost.TimelineItem, error)
}, feedCache interface {
	AddPostInboxItems(context.Context, []int64, *domainpost.TimelineItem, int64) error
}) *FollowFeedBackfiller {
	return &FollowFeedBackfiller{postRepo: postRepo, feedCache: feedCache}
}
func (b *FollowFeedBackfiller) CountFollowers(ctx context.Context, authorID int64) (int, error) {
	return b.postRepo.CountFollowers(ctx, authorID)
}
func (b *FollowFeedBackfiller) ListAuthorRecentPosts(ctx context.Context, authorID int64, limit int) ([]*domainpost.TimelineItem, error) {
	return b.postRepo.ListRecentTimelineItems(ctx, authorID, limit)
}
func (b *FollowFeedBackfiller) AddPostInboxItems(ctx context.Context, userIDs []int64, item *domainpost.TimelineItem, maxLen int64) error {
	return b.feedCache.AddPostInboxItems(ctx, userIDs, item, maxLen)
}

func HealthCheck(c *gin.Context) { c.JSON(200, gin.H{"message": "All is well"}) }
