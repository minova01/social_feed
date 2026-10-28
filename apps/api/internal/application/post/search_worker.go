package applicationpost

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const publishedStatus = "2"

type PostChangeConsumer interface {
	ConsumePostChanges(ctx context.Context, handler func(context.Context, [][]byte) error) error
}

type PostIndexChange struct {
	PostID      int64
	Content     string
	PublishedAt int64
	Published   bool
}

type SearchIndexWriter interface {
	ApplyPostChanges(ctx context.Context, changes []PostIndexChange) error
}

type SearchWorker struct {
	consumer PostChangeConsumer
	index    SearchIndexWriter
}

type canalMessage struct {
	Table string              `json:"table"`
	IsDDL bool                `json:"isDdl"`
	Type  string              `json:"type"`
	Data  []map[string]string `json:"data"`
}

func NewSearchWorker(consumer PostChangeConsumer, index SearchIndexWriter) *SearchWorker {
	return &SearchWorker{consumer: consumer, index: index}
}

func (w *SearchWorker) Start(ctx context.Context) error {
	return w.consumer.ConsumePostChanges(ctx, w.HandlePostChanged)
}

func (w *SearchWorker) HandlePostChanged(ctx context.Context, payloads [][]byte) error {
	changes := make(map[int64]PostIndexChange)
	order := make([]int64, 0, len(payloads))
	for _, payload := range payloads {
		parsed, err := parsePostChanges(payload)
		if err != nil {
			log.Printf("ignore invalid post cdc message: %v", err)
			continue
		}
		for _, change := range parsed {
			if _, exists := changes[change.PostID]; !exists {
				order = append(order, change.PostID)
			}
			changes[change.PostID] = change
		}
	}
	batch := make([]PostIndexChange, 0, len(changes))
	for _, postID := range order {
		batch = append(batch, changes[postID])
	}
	if len(batch) == 0 {
		return nil
	}
	return w.index.ApplyPostChanges(ctx, batch)
}

func parsePostChanges(payload []byte) ([]PostIndexChange, error) {
	var message canalMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return nil, err
	}
	if message.IsDDL || message.Table != "post" {
		return nil, nil
	}
	changes := make([]PostIndexChange, 0, len(message.Data))
	for _, row := range message.Data {
		postID, err := strconv.ParseInt(row["id"], 10, 64)
		if err != nil || postID <= 0 {
			return nil, fmt.Errorf("invalid post id %q", row["id"])
		}
		change := PostIndexChange{PostID: postID}
		if strings.EqualFold(message.Type, "DELETE") || row["status"] != publishedStatus {
			changes = append(changes, change)
			continue
		}
		publishedAt, err := strconv.ParseInt(row["published_at"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid published_at for post %d", postID)
		}
		change.Content = row["content"]
		change.PublishedAt = publishedAt
		change.Published = true
		changes = append(changes, change)
	}
	return changes, nil
}
