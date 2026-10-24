package infracache

import (
	"context"
	"encoding/json"
	"fmt"
	domainpost "social/internal/domain/post"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const postFeedRemainderTTL = 24 * time.Hour

func (c *FeedCache) GetPostFeedRemainder(ctx context.Context, userID int64) (*domainpost.FeedRemainder, bool, error) {
	content, err := c.client.Get(ctx, postFeedRemainderKey(userID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var remainder domainpost.FeedRemainder
	if err := json.Unmarshal(content, &remainder); err != nil {
		return nil, false, err
	}
	return &remainder, true, nil
}

func (c *FeedCache) SetPostFeedRemainder(ctx context.Context, userID int64, remainder *domainpost.FeedRemainder) error {
	content, err := json.Marshal(remainder)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, postFeedRemainderKey(userID), content, postFeedRemainderTTL).Err()
}

func (c *FeedCache) DeletePostFeedRemainder(ctx context.Context, userID int64) error {
	return c.client.Del(ctx, postFeedRemainderKey(userID)).Err()
}

func (c *FeedCache) ListPostFollowingWindow(ctx context.Context, viewerID int64, authorIDs []int64, after domainpost.TimelineCursor, cutoffAt int64, limit int) (*domainpost.TimelineBatch, *domainpost.TimelineBatch, bool, error) {
	return c.listPostFollowing(ctx, viewerID, authorIDs, after, cutoffAt, limit, false)
}

func (c *FeedCache) ListPostFollowingRecent(ctx context.Context, viewerID int64, authorIDs []int64, cutoffAt int64, limit int) ([]*domainpost.TimelineItem, []*domainpost.TimelineItem, bool, error) {
	inbox, outbox, hasIndex, err := c.listPostFollowing(ctx, viewerID, authorIDs, domainpost.TimelineCursor{}, cutoffAt, limit, true)
	if err != nil {
		return nil, nil, false, err
	}
	return inbox.Items, outbox.Items, hasIndex, nil
}

func (c *FeedCache) listPostFollowing(ctx context.Context, viewerID int64, authorIDs []int64, after domainpost.TimelineCursor, cutoffAt int64, limit int, recent bool) (*domainpost.TimelineBatch, *domainpost.TimelineBatch, bool, error) {
	keys := []string{postFollowingInboxKey(viewerID)}
	for _, authorID := range authorIDs {
		keys = append(keys, postFollowingAuthorOutboxKey(authorID))
	}

	pipe := c.client.Pipeline()
	existsCommands := make([]*redis.IntCmd, 0, len(keys))
	rangeCommands := make([]*redis.StringSliceCmd, 0, len(keys))
	for _, key := range keys {
		existsCommands = append(existsCommands, pipe.Exists(ctx, key))
		if recent {
			rangeCommands = append(rangeCommands, pipe.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
				Min:    "-inf",
				Max:    fmt.Sprintf("%d", cutoffAt),
				Offset: 0,
				Count:  int64(limit + 1),
			}))
			continue
		}
		rangeCommands = append(rangeCommands, pipe.ZRangeByScore(ctx, key, &redis.ZRangeBy{
			Min:    fmt.Sprintf("%d", after.PublishedAt),
			Max:    fmt.Sprintf("%d", cutoffAt),
			Offset: 0,
			Count:  int64(limit + 1),
		}))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, nil, false, err
	}

	hasIndex := false
	for _, command := range existsCommands {
		if count, err := command.Result(); err != nil {
			return nil, nil, false, err
		} else if count > 0 {
			hasIndex = true
		}
	}

	inboxItems, err := postTimelineItems(rangeCommands[:1], after, cutoffAt, limit, recent)
	if err != nil {
		return nil, nil, false, err
	}
	outboxItems, err := postTimelineItems(rangeCommands[1:], after, cutoffAt, limit, recent)
	if err != nil {
		return nil, nil, false, err
	}
	return &domainpost.TimelineBatch{Items: inboxItems, Capped: len(inboxItems) == limit},
		&domainpost.TimelineBatch{Items: outboxItems, Capped: len(outboxItems) == limit},
		hasIndex,
		nil
}

func postTimelineItems(commands []*redis.StringSliceCmd, after domainpost.TimelineCursor, cutoffAt int64, limit int, recent bool) ([]*domainpost.TimelineItem, error) {
	items := make([]*domainpost.TimelineItem, 0)
	seen := map[int64]struct{}{}
	for _, command := range commands {
		members, err := command.Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		for _, member := range members {
			var item domainpost.TimelineItem
			if json.Unmarshal([]byte(member), &item) != nil || item.PostID <= 0 {
				continue
			}
			if !recent && !timelineItemWithin(&item, after, cutoffAt) {
				continue
			}
			if _, ok := seen[item.PostID]; ok {
				continue
			}
			seen[item.PostID] = struct{}{}
			items = append(items, &item)
		}
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].PublishedAt == items[j].PublishedAt {
			return items[i].PostID < items[j].PostID
		}
		return items[i].PublishedAt < items[j].PublishedAt
	})
	if recent {
		reverseTimelineItems(items)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func timelineItemWithin(item *domainpost.TimelineItem, after domainpost.TimelineCursor, cutoffAt int64) bool {
	if item.PublishedAt > cutoffAt || item.PublishedAt < after.PublishedAt {
		return false
	}
	return item.PublishedAt > after.PublishedAt || item.PostID > after.PostID
}

func reverseTimelineItems(items []*domainpost.TimelineItem) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func postFeedRemainderKey(userID int64) string {
	return fmt.Sprintf("feed:following:remainder:v1:%d", userID)
}
