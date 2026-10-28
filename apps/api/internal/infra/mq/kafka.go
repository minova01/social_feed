package inframq

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	applicationpost "social/internal/application/post"
	domainpost "social/internal/domain/post"
	infraconfig "social/internal/infra/config"
	inframetrics "social/internal/infra/metrics"

	"github.com/segmentio/kafka-go"
)

const (
	postEventsTopic = "social.post.events"
	postCDCTopic    = "social.post.cdc"
	feedGroup       = "social-feed-worker"
	searchGroup     = "social-search-worker"
	cdcBatchSize    = 200
	cdcBatchWait    = 100 * time.Millisecond
)

var ErrInvalidKafkaConfig = errors.New("invalid kafka config")
var ErrUnsupportedPostEvent = errors.New("unsupported post event")

type Kafka struct {
	brokers []string
	writer  *kafka.Writer
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
	return &Kafka{
		brokers: brokers,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        postEventsTopic,
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireAll,
		},
	}, nil
}

func (k *Kafka) Close() error {
	if k == nil || k.writer == nil {
		return nil
	}
	return k.writer.Close()
}

func (k *Kafka) Publish(ctx context.Context, eventType string, postID int64) error {
	if eventType != domainpost.EventPublished {
		return ErrUnsupportedPostEvent
	}
	body, err := json.Marshal(applicationpost.NewPostEvent(postID))
	if err != nil {
		return err
	}
	return k.writer.WriteMessages(ctx, kafka.Message{Key: []byte(strconv.FormatInt(postID, 10)), Value: body})
}

func (k *Kafka) ConsumePostPublished(ctx context.Context, handler func(context.Context, *applicationpost.PostEvent) error) error {
	reader := k.reader(postEventsTopic, feedGroup)
	go func() {
		defer reader.Close()
		for {
			message, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("consume post event failed: %v", err)
				time.Sleep(time.Second)
				continue
			}
			start := time.Now()
			var event applicationpost.PostEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				inframetrics.ObserveWorkerJob("kafka_post_published_decode", time.Since(start), err)
				_ = reader.CommitMessages(ctx, message)
				continue
			}
			for {
				err = handler(ctx, &event)
				inframetrics.ObserveWorkerJob("kafka_post_published_consume", time.Since(start), err)
				if err == nil {
					_ = reader.CommitMessages(ctx, message)
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

func (k *Kafka) ConsumePostChanges(ctx context.Context, handler func(context.Context, [][]byte) error) error {
	reader := k.reader(postCDCTopic, searchGroup)
	go func() {
		defer reader.Close()
		for {
			messages, err := fetchBatch(ctx, reader)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("consume post cdc failed: %v", err)
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
				inframetrics.ObserveWorkerJob("kafka_post_cdc_consume", time.Since(start), err)
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
