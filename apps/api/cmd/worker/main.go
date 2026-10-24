package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"syscall"

	applicationpost "social/internal/application/post"
	infracache "social/internal/infra/cache"
	infraconfig "social/internal/infra/config"
	infradatabase "social/internal/infra/database"
	inframetrics "social/internal/infra/metrics"
	inframq "social/internal/infra/mq"
	migration "social/internal/infra/persistence/migration"
	infrapost "social/internal/infra/persistence/post"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg, err := infraconfig.Load()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	if cfg.RabbitMQ.URL == "" {
		log.Fatal("rabbitmq url is required for worker")
	}
	if cfg.Redis.Addr == "" {
		log.Fatal("redis addr is required for worker")
	}

	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		log.Fatalf("init database failed: %v", err)
	}
	defer closeSQL(sqlDB)

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Fatalf("init gorm failed: %v", err)
	}
	if err := migration.AutoMigrate(gormDB); err != nil {
		log.Fatalf("auto migrate failed: %v", err)
	}

	rabbitMQ, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		log.Fatalf("init rabbitmq failed: %v", err)
	}
	defer closeRabbitMQ(rabbitMQ)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := inframetrics.RunServer(ctx, ":9091"); err != nil {
			log.Printf("metrics server failed: %v", err)
		}
	}()

	if err := startWorkers(ctx, cfg, gormDB, rabbitMQ); err != nil {
		log.Fatalf("start workers failed: %v", err)
	}
	log.Println("social worker is running")
	<-ctx.Done()
	log.Println("social worker stopped")
}

func startWorkers(ctx context.Context, cfg *infraconfig.Config, gormDB *gorm.DB, rabbitMQ *inframq.RabbitMQ) error {
	redisClient := infracache.NewRedisClient(cfg.Redis)
	feedCache := infracache.NewFeedCache(redisClient)

	postRepo := infrapost.New(gormDB)
	postFanoutWorker := applicationpost.NewFanoutWorker(postRepo, rabbitMQ, feedCache)
	return postFanoutWorker.Start(ctx)
}

func closeSQL(db *sql.DB) {
	if db != nil {
		_ = db.Close()
	}
}

func closeRabbitMQ(rabbitMQ *inframq.RabbitMQ) {
	if rabbitMQ != nil {
		_ = rabbitMQ.Close()
	}
}
