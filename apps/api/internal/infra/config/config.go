package infraconfig

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var ErrMissingEnvironment = errors.New("required environment variable is missing")
var ErrInvalidEnvironment = errors.New("environment variable has an invalid value")

// Load 从进程环境变量读取应用配置。
func Load() (*Config, error) {
	port, err := intValue("APP_PORT", 8080)
	if err != nil {
		return nil, err
	}
	mysqlPort, err := intValue("MYSQL_PORT", 3306)
	if err != nil {
		return nil, err
	}
	redisDB, err := intValue("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}
	if port <= 0 || mysqlPort <= 0 || redisDB < 0 {
		return nil, ErrInvalidEnvironment
	}

	cfg := &Config{
		Port: port,
		JWT: JWTConfig{
			Secret:    value("JWT_SECRET", ""),
			AccessTTL: value("JWT_ACCESS_TTL", "15m"),
		},
		Internal: InternalConfig{Token: value("INTERNAL_TOKEN", "")},
		Database: DatabaseConfig{
			Host:     value("MYSQL_HOST", ""),
			Port:     mysqlPort,
			User:     value("MYSQL_USER", ""),
			Password: value("MYSQL_PASSWORD", ""),
			Name:     value("MYSQL_DATABASE", ""),
		},
		Redis: RedisConfig{
			Addr:     value("REDIS_ADDR", ""),
			Password: value("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		Kafka: KafkaConfig{Brokers: value("KAFKA_BROKERS", "")},
		Search: SearchConfig{
			URL:   value("ELASTICSEARCH_URL", ""),
			Index: value("ELASTICSEARCH_INDEX", "social_posts"),
		},
		Upload: UploadConfig{
			Root:          value("UPLOAD_ROOT", "./uploads"),
			PublicBaseURL: value("PUBLIC_UPLOAD_BASE_URL", "/uploads"),
		},
	}

	required := []struct {
		name  string
		value string
	}{
		{name: "JWT_SECRET", value: cfg.JWT.Secret},
		{name: "INTERNAL_TOKEN", value: cfg.Internal.Token},
		{name: "MYSQL_HOST", value: cfg.Database.Host},
		{name: "MYSQL_USER", value: cfg.Database.User},
		{name: "MYSQL_PASSWORD", value: cfg.Database.Password},
		{name: "MYSQL_DATABASE", value: cfg.Database.Name},
		{name: "KAFKA_BROKERS", value: cfg.Kafka.Brokers},
		{name: "ELASTICSEARCH_URL", value: cfg.Search.URL},
	}
	for _, item := range required {
		if item.value == "" {
			return nil, fmt.Errorf("%w: %s", ErrMissingEnvironment, item.name)
		}
	}
	return cfg, nil
}

func value(name string, fallback string) string {
	configured := strings.TrimSpace(os.Getenv(name))
	if configured == "" {
		return fallback
	}
	return configured
}

func intValue(name string, fallback int) (int, error) {
	configured := strings.TrimSpace(os.Getenv(name))
	if configured == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(configured)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidEnvironment, name)
	}
	return parsed, nil
}
