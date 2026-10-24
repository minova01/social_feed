package applicationpost

import (
	"context"
	"errors"

	domainpost "social/internal/domain/post"
)

type SearchEventConsumer interface {
	ConsumePostSearch(ctx context.Context, handler func(context.Context, *PostEvent) error) error
}

type SearchIndexWriter interface {
	UpsertPost(ctx context.Context, postID int64, content string, publishedAt int64) error
	DeletePost(ctx context.Context, postID int64) error
}

type SearchWorker struct {
	repo     domainpost.Repository
	consumer SearchEventConsumer
	index    SearchIndexWriter
}

func NewSearchWorker(repo domainpost.Repository, consumer SearchEventConsumer, index SearchIndexWriter) *SearchWorker {
	return &SearchWorker{repo: repo, consumer: consumer, index: index}
}

func (w *SearchWorker) Start(ctx context.Context) error {
	return w.consumer.ConsumePostSearch(ctx, w.HandlePostChanged)
}

func (w *SearchWorker) HandlePostChanged(ctx context.Context, event *PostEvent) error {
	if event == nil || event.PostID <= 0 {
		return nil
	}
	post, err := w.repo.FindByIDAnyStatus(ctx, event.PostID)
	if errors.Is(err, domainpost.ErrPostNotFound) {
		return w.index.DeletePost(ctx, event.PostID)
	}
	if err != nil {
		return err
	}
	if post.Status != domainpost.StatusPublished {
		return w.index.DeletePost(ctx, post.ID)
	}
	return w.index.UpsertPost(ctx, post.ID, post.Content, post.PublishedAt)
}
