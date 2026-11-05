package inframq

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	infraconfig "social/internal/infra/config"
	inframetrics "social/internal/infra/metrics"

	"github.com/segmentio/kafka-go"
)

const (
	postCDCTopic = "social.post.cdc"
	feedGroup    = "social-feed-worker"
	searchGroup  = "social-search-worker"
	cdcBatchSize = 200
	cdcBatchWait = 100 * time.Millisecond
)

var ErrInvalidKafkaConfig = errors.New("invalid kafka config")

type Kafka struct {
	brokers []string
}

func NewKafka(cfg infraconfig.KafkaConfig) (*Kafka, error) {
	brokers := splitBrokers(cfg.Brokers)
	if len(brokers) == 0 {
		return nil, ErrInvalidKafkaConfig
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return nil, err
	}
	_ = conn.Close()
	return &Kafka{brokers: brokers}, nil
}

func (k *Kafka) ConsumePostPublished(ctx context.Context, handler func(context.Context, [][]byte) error) error {
	return k.consumePostChanges(ctx, feedGroup, "kafka_post_feed_consume", handler)
}

func (k *Kafka) ConsumePostChanges(ctx context.Context, handler func(context.Context, [][]byte) error) error {
	return k.consumePostChanges(ctx, searchGroup, "kafka_post_search_consume", handler)
}

func (k *Kafka) consumePostChanges(ctx context.Context, group string, metric string, handler func(context.Context, [][]byte) error) error {
	reader := k.reader(postCDCTopic, group)
	go func() {
		defer reader.Close()
		for {
			messages, err := fetchBatch(ctx, reader)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("consume post cdc failed: group=%s error=%v", group, err)
				time.Sleep(time.Second)
				continue
			}
			payloads := make([][]byte, 0, len(messages))
			for _, message := range messages {
				payloads = append(payloads, message.Value)
			}
			start := time.Now()
			for {
				err = handler(ctx, payloads)
				inframetrics.ObserveWorkerJob(metric, time.Since(start), err)
				if err == nil {
					_ = reader.CommitMessages(ctx, messages...)
					break
				}
				if !waitRetry(ctx) {
					return
				}
			}
		}
	}()
	return nil
}

func (k *Kafka) reader(topic string, group string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  k.brokers,
		Topic:    topic,
		GroupID:  group,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
}

func fetchBatch(ctx context.Context, reader *kafka.Reader) ([]kafka.Message, error) {
	first, err := reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}
	messages := []kafka.Message{first}
	batchCtx, cancel := context.WithTimeout(ctx, cdcBatchWait)
	defer cancel()
	for len(messages) < cdcBatchSize {
		message, err := reader.FetchMessage(batchCtx)
		if errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func splitBrokers(value string) []string {
	parts := strings.Split(value, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		if broker := strings.TrimSpace(part); broker != "" {
			brokers = append(brokers, broker)
		}
	}
	return brokers
}

func waitRetry(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
