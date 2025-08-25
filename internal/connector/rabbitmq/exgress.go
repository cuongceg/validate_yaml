package rabbitmq

import (
	"context"
	"fmt"
	"time"

	core "github.com/cuongceg/validate_yaml/internal/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

type egress struct {
	targetName string
	exchange   string
	routingKey string

	persistent         bool
	defaultContentType string
	publishTimeout     time.Duration

	conn *amqp.Connection
	ch   *amqp.Channel
}

func newEgress(conn *amqp.Connection, cfg EgressConfig) (core.Egress, error) {
	if cfg.TargetName == "" {
		return nil, fmt.Errorf("egress requires targetName")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("egress channel: %w", err)
	}
	return &egress{
		targetName:         cfg.TargetName,
		exchange:           cfg.Exchange,
		routingKey:         cfg.RoutingKey,
		persistent:         cfg.Persistent,
		defaultContentType: cfg.DefaultContentType,
		publishTimeout:     cfg.PublishTimeout,
		conn:               conn,
		ch:                 ch,
	}, nil
}

func (e *egress) TargetName() string { return e.targetName }

func (e *egress) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	// Lấy content-type từ meta (nếu có), fallback về defaultContentType
	ct := e.defaultContentType
	if meta != nil {
		if v, ok := meta["content-type"]; ok && v != "" {
			ct = v
		}
	}

	// Xây headers từ meta
	hdrs := amqp.Table{}
	for k, v := range meta {
		// đừng lặp lại content-type trong headers
		if k == "content-type" {
			continue
		}
		hdrs[k] = v
	}

	pub := amqp.Publishing{
		ContentType: ct,
		Body:        msg,
		Headers:     hdrs,
		// Persistent?
		DeliveryMode: func() uint8 {
			if e.persistent {
				return amqp.Persistent
			}
			return amqp.Transient
		}(),
	}

	// Tôn trọng deadline trong ctx, nếu không có dùng publishTimeout (nếu > 0)
	var cancel func()
	if _, has := ctx.Deadline(); !has && e.publishTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, e.publishTimeout)
		defer cancel()
	}

	return e.ch.PublishWithContext(ctx,
		e.exchange,
		e.routingKey,
		false, // mandatory
		false, // immediate (RabbitMQ bỏ, driver vẫn yêu cầu bool)
		pub,
	)
}

func (e *egress) Close() error {
	return e.ch.Close()
}
