package applicationpost

import (
	"context"
	"encoding/json"
	"errors"
	domainpost "social/internal/domain/post"
	"strings"
)

const defaultFollowerBatchSize = 500
const defaultInboxMaxLen int64 = 1000
const defaultOutboxMaxLen int64 = 500

type PostPublishedConsumer interface {
	ConsumePostPublished(ctx context.Context, handler func(context.Context, [][]byte) error) error
}

type FanoutRepository interface {
	FindByID(ctx context.Context, postID int64) (*domainpost.Post, error)
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	ListFollowerIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error)
}

type TimelineIndex interface {
	AddPostInboxItems(ctx context.Context, userIDs []int64, item *domainpost.TimelineItem, maxLen int64) error
	AddPostAuthorOutboxItem(ctx context.Context, item *domainpost.TimelineItem, maxLen int64) error
}

type FanoutWorker struct {
	repo     FanoutRepository
	consumer PostPublishedConsumer
	index    TimelineIndex
}

func NewFanoutWorker(repo FanoutRepository, consumer PostPublishedConsumer, index TimelineIndex) *FanoutWorker {
	return &FanoutWorker{repo: repo, consumer: consumer, index: index}
}

func (w *FanoutWorker) Start(ctx context.Context) error {
	return w.consumer.ConsumePostPublished(ctx, w.HandlePostChanges)
}

func (w *FanoutWorker) HandlePostChanges(ctx context.Context, payloads [][]byte) error {
	seen := make(map[int64]struct{})
	invalid := make([]int, 0)
	var invalidCause error
	for index, payload := range payloads {
		var message canalMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			invalid = append(invalid, index)
			invalidCause = err
			continue
		}
		if message.IsDDL || message.Table != "post" || !strings.EqualFold(message.Type, "INSERT") {
			continue
		}
		changes, err := parsePostChanges(payload)
		if err != nil {
			invalid = append(invalid, index)
			invalidCause = err
			continue
		}
		for _, change := range changes {
			if !change.Published {
				continue
			}
			if _, exists := seen[change.PostID]; exists {
				continue
			}
			seen[change.PostID] = struct{}{}
			if err := w.HandlePostPublished(ctx, NewPostEvent(change.PostID)); err != nil {
				return err
			}
		}
	}
	if len(invalid) > 0 {
		return &InvalidMessagesError{Indexes: invalid, Cause: invalidCause}
	}
	return nil
}

func (w *FanoutWorker) HandlePostPublished(ctx context.Context, event *PostEvent) error {
	if event == nil || event.PostID <= 0 {
		return nil
	}
	post, err := w.repo.FindByID(ctx, event.PostID)
	if errors.Is(err, domainpost.ErrPostNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	item := &domainpost.TimelineItem{
		PostID:      post.ID,
		AuthorID:    post.AuthorID,
		PublishedAt: post.PublishedAt,
	}
	followerCount, err := w.repo.CountFollowers(ctx, post.AuthorID)
	if err != nil {
		return err
	}
	if followerCount >= domainpost.BigCreatorFollowerThreshold {
		return w.index.AddPostAuthorOutboxItem(ctx, item, defaultOutboxMaxLen)
	}
	// 分批获取粉丝ID并写入收件箱
	var cursor int64
	for {
		followerIDs, err := w.repo.ListFollowerIDs(ctx, post.AuthorID, cursor, defaultFollowerBatchSize)
		if err != nil {
			return err
		}
		if len(followerIDs) == 0 {
			return nil
		}
		if err := w.index.AddPostInboxItems(ctx, followerIDs, item, defaultInboxMaxLen); err != nil {
			return err
		}
		cursor = followerIDs[len(followerIDs)-1]
		if len(followerIDs) < defaultFollowerBatchSize {
			return nil
		}
	}
}
