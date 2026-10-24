package inframq

import (
	"context"
	"encoding/json"
	"errors"
	applicationpost "social/internal/application/post"
	infraconfig "social/internal/infra/config"
	inframetrics "social/internal/infra/metrics"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const postExchange = "social.post"
const postPublishedQueue = "social.post.published"
const postPublishedRouting = "post.published"

var ErrEmptyRabbitMQURL = errors.New("rabbitmq url is empty")

type RabbitMQ struct {
	conn            *amqp.Connection
	publishChannel  *amqp.Channel
	consumerChannel *amqp.Channel
}

func NewRabbitMQ(cfg infraconfig.RabbitMQConfig) (*RabbitMQ, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrEmptyRabbitMQURL
	}
	conn, err := amqp.Dial(strings.TrimSpace(cfg.URL))
	if err != nil {
		return nil, err
	}
	publishChannel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	consumerChannel, err := conn.Channel()
	if err != nil {
		_ = publishChannel.Close()
		_ = conn.Close()
		return nil, err
	}
	client := &RabbitMQ{conn: conn, publishChannel: publishChannel, consumerChannel: consumerChannel}
	if err := client.ensureTopology(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	if r.consumerChannel != nil {
		_ = r.consumerChannel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
func (r *RabbitMQ) PublishPostPublished(ctx context.Context, event *applicationpost.PublishedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publishChannel.PublishWithContext(ctx, postExchange, postPublishedRouting, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Timestamp: time.Now(), Body: content})
}
func (r *RabbitMQ) ConsumePostPublished(ctx context.Context, handler func(context.Context, *applicationpost.PublishedEvent) error) error {
	deliveries, err := r.consumerChannel.ConsumeWithContext(ctx, postPublishedQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for delivery := range deliveries {
			start := time.Now()
			var event applicationpost.PublishedEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				inframetrics.ObserveWorkerJob("mq_post_published_decode", time.Since(start), err)
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, &event); err != nil {
				inframetrics.ObserveWorkerJob("mq_post_published_consume", time.Since(start), err)
				_ = delivery.Nack(false, true)
				continue
			}
			inframetrics.ObserveWorkerJob("mq_post_published_consume", time.Since(start), nil)
			_ = delivery.Ack(false)
		}
	}()
	return nil
}
func (r *RabbitMQ) ensureTopology() error {
	if err := r.publishChannel.ExchangeDeclare(postExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(postPublishedQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return r.publishChannel.QueueBind(postPublishedQueue, postPublishedRouting, postExchange, false, nil)
}
