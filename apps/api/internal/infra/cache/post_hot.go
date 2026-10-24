package infracache

import (
	"context"
	"fmt"
	domainpost "social/internal/domain/post"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const postHotWindowMinutes = 60
const postHotMinuteTTL = 2 * time.Hour
const postHotWindowTTL = 2 * time.Minute

func (c *FeedCache) AddPostHotScore(ctx context.Context, postID int64, scoreDelta int, at time.Time) error {
	key := postHotMinuteKey(at)
	if err := c.client.ZIncrBy(ctx, key, float64(scoreDelta), strconv.FormatInt(postID, 10)).Err(); err != nil {
		return err
	}
	return c.client.Expire(ctx, key, postHotMinuteTTL).Err()
}

func (c *FeedCache) ListPostHotWindow(ctx context.Context, offset int, limit int) ([]*domainpost.HotItem, error) {
	windowEnd := time.Now().UTC().Truncate(time.Minute)
	windowKey := postHotWindowKey(windowEnd)
	exists, err := c.client.Exists(ctx, windowKey).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		keys := make([]string, 0, postHotWindowMinutes)
		for minute := 0; minute < postHotWindowMinutes; minute++ {
			keys = append(keys, postHotMinuteKey(windowEnd.Add(-time.Duration(minute)*time.Minute)))
		}
		if _, err := c.client.ZUnionStore(ctx, windowKey, &redis.ZStore{Keys: keys, Aggregate: "SUM"}).Result(); err != nil {
			return nil, err
		}
		pipe := c.client.Pipeline()
		pipe.ZRemRangeByScore(ctx, windowKey, "-inf", "0")
		pipe.Expire(ctx, windowKey, postHotWindowTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
	}
	values, err := c.client.ZRevRangeWithScores(ctx, windowKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	items := make([]*domainpost.HotItem, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			continue
		}
		postID, err := strconv.ParseInt(member, 10, 64)
		if err != nil || postID <= 0 {
			continue
		}
		items = append(items, &domainpost.HotItem{PostID: postID, HotScore: int(value.Score)})
	}
	return items, nil
}

func postHotMinuteKey(at time.Time) string {
	return fmt.Sprintf("feed:hot:minute:v2:%s", at.UTC().Format("200601021504"))
}

func postHotWindowKey(windowEnd time.Time) string {
	return fmt.Sprintf("feed:hot:window:v2:%d", windowEnd.UnixMilli())
}
