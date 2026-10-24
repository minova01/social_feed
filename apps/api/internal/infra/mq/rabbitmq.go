package inframq

import (
	"context"
	"encoding/json"
	"errors"
	applicationpost "social/internal/application/post"
	domainpost "social/internal/domain/post"
	infraconfig "social/internal/infra/config"
	inframetrics "social/internal/infra/metrics"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const postExchange = "social.post"
const postPublishedQueue = "social.post.published"
const postSearchQueue = "social.post.search"
const postPublishedRouting = "post.published"

var ErrEmptyRabbitMQURL = errors.New("rabbitmq url is empty")
var ErrPostEventPublishNotConfirmed = errors.New("post event publish not confirmed")
var ErrUnsupportedPostEvent = errors.New("unsupported post event")

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
	if err := publishChannel.Confirm(false); err != nil {
		_ = publishChannel.Close()
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
func (r *RabbitMQ) Publish(ctx context.Context, eventType string, postID int64) error {
	if eventType != domainpost.EventPublished && eventType != domainpost.EventDeleted {
		return ErrUnsupportedPostEvent
	}
	content, err := json.Marshal(applicationpost.NewPostEvent(postID))
	if err != nil {
		return err
	}
	confirmation, err := r.publishChannel.PublishWithDeferredConfirmWithContext(ctx, postExchange, eventType, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Timestamp: time.Now(), Body: content})
	if err != nil {
		return err
	}
	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrPostEventPublishNotConfirmed
	}
	return nil
}
func (r *RabbitMQ) ConsumePostPublished(ctx context.Context, handler func(context.Context, *applicationpost.PostEvent) error) error {
	return r.consumePostEvents(ctx, postPublishedQueue, "mq_post_published", handler)
}

func (r *RabbitMQ) ConsumePostSearch(ctx context.Context, handler func(context.Context, *applicationpost.PostEvent) error) error {
	return r.consumePostEvents(ctx, postSearchQueue, "mq_post_search", handler)
}

func (r *RabbitMQ) consumePostEvents(ctx context.Context, queue string, job string, handler func(context.Context, *applicationpost.PostEvent) error) error {
	deliveries, err := r.consumerChannel.ConsumeWithContext(ctx, queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for delivery := range deliveries {
			start := time.Now()
			var event applicationpost.PostEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				inframetrics.ObserveWorkerJob(job+"_decode", time.Since(start), err)
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, &event); err != nil {
				inframetrics.ObserveWorkerJob(job+"_consume", time.Since(start), err)
				_ = delivery.Nack(false, true)
				continue
			}
			inframetrics.ObserveWorkerJob(job+"_consume", time.Since(start), nil)
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
	if err := r.publishChannel.QueueBind(postPublishedQueue, postPublishedRouting, postExchange, false, nil); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(postSearchQueue, true, false, false, false, nil); err != nil {
		return err
	}
	for _, routingKey := range []string{domainpost.EventPublished, domainpost.EventDeleted} {
		if err := r.publishChannel.QueueBind(postSearchQueue, routingKey, postExchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}
