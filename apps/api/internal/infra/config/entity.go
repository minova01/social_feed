package infraconfig

// Config 是应用启动配置的根结构，值由进程环境变量提供。
type Config struct {
	Port     int
	JWT      JWTConfig
	Internal InternalConfig
	Database DatabaseConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Upload   UploadConfig
}

// JWTConfig 保存 JWT 签名密钥和访问 token 有效期。
type JWTConfig struct {
	Secret    string
	AccessTTL string
}

// InternalConfig 保存内部接口服务鉴权配置。
type InternalConfig struct {
	Token string
}

// DatabaseConfig 保存 MySQL 连接参数。
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// RedisConfig 保存 Redis 连接参数，用于 Feed 读缓存。
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// RabbitMQConfig 保存 RabbitMQ 连接和队列配置。
type RabbitMQConfig struct {
	URL string
}

// UploadConfig 保存本地上传目录和对外访问前缀。
type UploadConfig struct {
	Root          string
	PublicBaseURL string
}
