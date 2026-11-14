package infracache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const postRateLimitTTL = time.Hour

var postRateLimitScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local member = ARGV[2]
local minute_start = now - 60000
local hour_start = now - 3600000

redis.call('ZREMRANGEBYSCORE', key, '-inf', hour_start)

if redis.call('ZSCORE', key, member) then
    return {1, 0}
end

local minute_count = redis.call('ZCOUNT', key, minute_start, '+inf')
if minute_count >= 5 then
    local oldest = redis.call('ZRANGEBYSCORE', key, minute_start, '+inf', 'WITHSCORES', 'LIMIT', 0, 1)
    return {0, math.max(1, tonumber(oldest[2]) + 60000 - now)}
end

local hour_count = redis.call('ZCARD', key)
if hour_count >= 30 then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    return {0, math.max(1, tonumber(oldest[2]) + 3600000 - now)}
end

redis.call('ZADD', key, now, member)
redis.call('EXPIRE', key, ARGV[3])
return {1, 0}
`)

type PostRateLimiter struct {
	client *redis.Client
}

func NewPostRateLimiter(client *redis.Client) *PostRateLimiter {
	return &PostRateLimiter{client: client}
}

func (l *PostRateLimiter) AllowPost(ctx context.Context, authorID int64, idempotencyKey string) (bool, time.Duration, error) {
	now := time.Now()
	member, err := postRateLimitMember(idempotencyKey)
	if err != nil {
		return false, 0, err
	}
	result, err := postRateLimitScript.Run(
		ctx,
		l.client,
		[]string{fmt.Sprintf("post:rate-limit:%d", authorID)},
		now.UnixMilli(),
		member,
		int(postRateLimitTTL/time.Second),
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("unexpected post rate limit result")
	}
	allowed, err := redisInt64(result[0])
	if err != nil {
		return false, 0, err
	}
	retryAfterMilliseconds, err := redisInt64(result[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, time.Duration(retryAfterMilliseconds) * time.Millisecond, nil
}

func postRateLimitMember(idempotencyKey string) (string, error) {
	if idempotencyKey != "" {
		return "idempotency:" + idempotencyKey, nil
	}
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "request:" + hex.EncodeToString(value), nil
}

func redisInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected redis integer type %T", value)
	}
}
