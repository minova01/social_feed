package inframq

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	infraconfig "social/internal/infra/config"
	inframetrics "social/internal/infra/metrics"

	"github.com/segmentio/kafka-go"
)

const (
	postCDCTopic   = "social.post.cdc"
	feedGroup      = "social-feed-worker"
	searchGroup    = "social-search-worker"
	feedDLQTopic   = "social.post.feed.dlq"
	searchDLQTopic = "social.post.search.dlq"
	cdcBatchSize   = 200
	cdcBatchWait   = 100 * time.Millisecond
	maxAttempts    = 5
	retryBase      = time.Second
	retryMax       = 30 * time.Second
)

var ErrInvalidKafkaConfig = errors.New("invalid kafka config")

type Kafka struct {
	brokers []string
	writer  *kafka.Writer
}

type failedMessagesError interface {
	error
	FailedMessageIndexes() []int
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
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireAll,
		},
	}, nil
}

func (k *Kafka) ConsumePostPublished(ctx context.Context, handler func(context.Context, [][]byte) error) error {
	return k.consumePostChanges(ctx, feedGroup, feedDLQTopic, "kafka_post_feed_consume", handler)
}

func (k *Kafka) ConsumePostChanges(ctx context.Context, handler func(context.Context, [][]byte) error) error {
	return k.consumePostChanges(ctx, searchGroup, searchDLQTopic, "kafka_post_search_consume", handler)
}

func (k *Kafka) Close() error {
	return k.writer.Close()
}

func (k *Kafka) consumePostChanges(ctx context.Context, group string, dlqTopic string, metric string, handler func(context.Context, [][]byte) error) error {
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
			var failed []kafka.Message
			var handlerErr error
			attempts := 0
			for attempts < maxAttempts {
				attempts++
				start := time.Now()
				err = handler(ctx, payloads)
				inframetrics.ObserveWorkerJob(metric, time.Since(start), err)
				handlerErr = err
				if err == nil {
					break
				}
				var invalid failedMessagesError
				if errors.As(err, &invalid) {
					failed = selectMessages(messages, invalid.FailedMessageIndexes())
					break
				}
				if attempts < maxAttempts && !waitRetry(ctx, attempts) {
					return
				}
			}
			if handlerErr != nil {
				if len(failed) == 0 {
					failed = messages
				}
				for !k.writeDLQ(ctx, dlqTopic, group, failed, attempts, handlerErr) {
					if ctx.Err() != nil || !waitRetry(ctx, attempts) {
						return
					}
				}
			}
			if err := reader.CommitMessages(ctx, messages...); err != nil && ctx.Err() == nil {
				log.Printf("commit post cdc failed: group=%s error=%v", group, err)
			}
		}
	}()
	return nil
}

func (k *Kafka) writeDLQ(ctx context.Context, topic string, group string, messages []kafka.Message, attempts int, cause error) bool {
	dlqMessages := make([]kafka.Message, 0, len(messages))
	for _, message := range messages {
		dlqMessages = append(dlqMessages, kafka.Message{
			Topic: topic,
			Key:   message.Key,
			Value: message.Value,
			Headers: []kafka.Header{
				{Key: "source_topic", Value: []byte(message.Topic)},
				{Key: "consumer_group", Value: []byte(group)},
				{Key: "source_partition", Value: []byte(strconv.Itoa(message.Partition))},
				{Key: "source_offset", Value: []byte(strconv.FormatInt(message.Offset, 10))},
				{Key: "attempts", Value: []byte(strconv.Itoa(attempts))},
				{Key: "error", Value: []byte(cause.Error())},
			},
		})
	}
	if err := k.writer.WriteMessages(ctx, dlqMessages...); err != nil {
		log.Printf("write post cdc dlq failed: topic=%s group=%s error=%v", topic, group, err)
		return false
	}
	log.Printf("post cdc moved to dlq: topic=%s group=%s count=%d error=%v", topic, group, len(messages), cause)
	return true
}

func selectMessages(messages []kafka.Message, indexes []int) []kafka.Message {
	selected := make([]kafka.Message, 0, len(indexes))
	seen := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len(messages) {
			continue
		}
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		selected = append(selected, messages[index])
	}
	return selected
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

func waitRetry(ctx context.Context, attempt int) bool {
	delay := retryBase << max(attempt-1, 0)
	if delay > retryMax {
		delay = retryMax
	}
	delay += time.Duration(rand.Int63n(int64(delay/2) + 1))
	if delay > retryMax {
		delay = retryMax
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
