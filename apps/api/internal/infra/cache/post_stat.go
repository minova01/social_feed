package infracache

import (
	"context"
	"fmt"
	domainpost "social/internal/domain/post"
	domainpostinteraction "social/internal/domain/postinteraction"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const postStatInitializationLockTTL = 3 * time.Second
const postStatInitializationWait = 25 * time.Millisecond
const postStatInitializationAttempts = 40

func (c *FeedCache) EnsurePostStat(ctx context.Context, postID int64, loader func(context.Context, int64) (*domainpost.Stat, error)) (*domainpost.Stat, error) {
	if stat, ok, err := c.getPostStat(ctx, postID); err != nil || ok {
		return stat, err
	}
	lockKey := postStatInitializationLockKey(postID)
	locked, err := c.client.SetNX(ctx, lockKey, "1", postStatInitializationLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if locked {
		stat, err := loader(ctx, postID)
		if err != nil {
			return nil, err
		}
		if err := c.initializePostStat(ctx, stat); err != nil {
			return nil, err
		}
		return stat, nil
	}
	for attempt := 0; attempt < postStatInitializationAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(postStatInitializationWait):
		}
		if stat, ok, err := c.getPostStat(ctx, postID); err != nil || ok {
			return stat, err
		}
	}
	return nil, fmt.Errorf("post stat initialization timed out")
}

func (c *FeedCache) ApplyPostActionDelta(ctx context.Context, postID int64, actionType string, delta int) (*domainpost.Stat, error) {
	key := postStatKey(postID)
	field := "favorite_count"
	if actionType == domainpostinteraction.ActionTypeLike {
		field = "like_count"
	}
	pipe := c.client.Pipeline()
	if delta != 0 {
		pipe.HIncrBy(ctx, key, field, int64(delta))
	}
	valuesCommand := pipe.HGetAll(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	values, err := valuesCommand.Result()
	if err != nil {
		return nil, err
	}
	return postStatFromValues(postID, values), nil
}

func (c *FeedCache) BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error) {
	stats := map[int64]*domainpost.Stat{}
	if len(postIDs) == 0 {
		return stats, nil
	}
	pipe := c.client.Pipeline()
	commands := make([]*redis.MapStringStringCmd, 0, len(postIDs))
	for _, postID := range postIDs {
		commands = append(commands, pipe.HGetAll(ctx, postStatKey(postID)))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	for index, command := range commands {
		postID := postIDs[index]
		values, err := command.Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		if len(values) == 0 {
			fallback := fallbacks[postID]
			if fallback != nil {
				locked, lockErr := c.client.SetNX(ctx, postStatInitializationLockKey(postID), "1", postStatInitializationLockTTL).Result()
				if lockErr == nil && locked {
					_ = c.initializePostStat(ctx, fallback)
				}
				stats[postID] = fallback
			}
			continue
		}
		stats[postID] = postStatFromValues(postID, values)
	}
	return stats, nil
}

func (c *FeedCache) getPostStat(ctx context.Context, postID int64) (*domainpost.Stat, bool, error) {
	values, err := c.client.HGetAll(ctx, postStatKey(postID)).Result()
	if err != nil {
		return nil, false, err
	}
	if _, likeOK := values["like_count"]; !likeOK {
		return nil, false, nil
	}
	if _, favoriteOK := values["favorite_count"]; !favoriteOK {
		return nil, false, nil
	}
	return postStatFromValues(postID, values), true, nil
}

func (c *FeedCache) initializePostStat(ctx context.Context, stat *domainpost.Stat) error {
	pipe := c.client.Pipeline()
	pipe.HSetNX(ctx, postStatKey(stat.PostID), "like_count", stat.LikeCount)
	pipe.HSetNX(ctx, postStatKey(stat.PostID), "favorite_count", stat.FavoriteCount)
	_, err := pipe.Exec(ctx)
	return err
}

func postStatFromValues(postID int64, values map[string]string) *domainpost.Stat {
	likeCount, _ := strconv.Atoi(values["like_count"])
	favoriteCount, _ := strconv.Atoi(values["favorite_count"])
	return &domainpost.Stat{PostID: postID, LikeCount: likeCount, FavoriteCount: favoriteCount}
}

func postStatKey(postID int64) string {
	return fmt.Sprintf("post:stat:v1:%d", postID)
}

func postStatInitializationLockKey(postID int64) string {
	return fmt.Sprintf("post:stat:init:lock:v1:%d", postID)
}
