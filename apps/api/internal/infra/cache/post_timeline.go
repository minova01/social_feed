package infracache

import (
	"context"
	"encoding/json"
	"fmt"
	domainpost "social/internal/domain/post"
	"time"

	"github.com/redis/go-redis/v9"
)

const postTimelineTTL = 30 * 24 * time.Hour

func (c *FeedCache) AddPostInboxItems(ctx context.Context, userIDs []int64, item *domainpost.TimelineItem, maxLen int64) error {
	member, err := json.Marshal(item)
	if err != nil {
		return err
	}
	pipe := c.client.Pipeline()
	for _, userID := range userIDs {
		key := postFollowingInboxKey(userID)
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(item.PublishedAt), Member: member})
		pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
		pipe.Expire(ctx, key, postTimelineTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *FeedCache) AddPostAuthorOutboxItem(ctx context.Context, item *domainpost.TimelineItem, maxLen int64) error {
	member, err := json.Marshal(item)
	if err != nil {
		return err
	}
	key := postFollowingAuthorOutboxKey(item.AuthorID)
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(item.PublishedAt), Member: member})
	pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
	pipe.Expire(ctx, key, postTimelineTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func postFollowingInboxKey(userID int64) string {
	return fmt.Sprintf("feed:following:inbox:v2:%d", userID)
}

func postFollowingAuthorOutboxKey(authorID int64) string {
	return fmt.Sprintf("feed:following:author:v2:%d", authorID)
}
