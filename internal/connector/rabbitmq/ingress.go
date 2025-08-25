package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	core "github.com/cuongceg/validate_yaml/internal/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

type ingress struct {
	sourceName string
	queue      string

	autoAck        bool
	prefetch       int
	prefetchGlobal bool
	requeueOnError bool
	consumerTag    string

	conn *amqp.Connection
	ch   *amqp.Channel

	stopped atomic.Bool
	doneCh  chan struct{}
}

func newIngress(conn *amqp.Connection, cfg IngressConfig) (core.Ingress, error) {
	if cfg.SourceName == "" || cfg.Queue == "" {
		return nil, fmt.Errorf("ingress requires sourceName and queue")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("ingress channel: %w", err)
	}
	ing := &ingress{
		sourceName:     cfg.SourceName,
		queue:          cfg.Queue,
		autoAck:        cfg.AutoAck,
		prefetch:       cfg.Prefetch,
		prefetchGlobal: cfg.PrefetchGlobal,
		requeueOnError: cfg.RequeueOnError,
		consumerTag:    cfg.ConsumerTag,
		conn:           conn,
		ch:             ch,
		doneCh:         make(chan struct{}),
	}
	return ing, nil
}

func (i *ingress) SourceName() string { return i.sourceName }

func (i *ingress) Start(ctx context.Context, h core.Handler) error {
	if i.prefetch > 0 {
		if err := i.ch.Qos(i.prefetch, 0, i.prefetchGlobal); err != nil {
			return fmt.Errorf("set QoS: %w", err)
		}
	}
	deliveries, err := i.ch.Consume(
		i.queue,
		i.consumerTag,
		i.autoAck,
		false, // exclusive
		false, // noLocal (RabbitMQ không dùng)
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	go func() {
		defer close(i.doneCh)
		for {
			select {
			case d, ok := <-deliveries:
				if !ok {
					return
				}

				// Build meta từ headers/properties
				meta := map[string]string{
					"content-type": d.ContentType,
					"exchange":     d.Exchange,
					"routing-key":  d.RoutingKey,
					"delivery-tag": fmt.Sprintf("%d", d.DeliveryTag),
				}
				for k, v := range d.Headers {
					// chuyển mọi header về string nếu có thể
					if s, ok := v.(string); ok {
						meta["hdr."+k] = s
					}
				}

				// Handler
				err := h(ctx, d.Body, meta)
				if i.autoAck {
					// autoAck -> broker đã ack ngay khi gửi, không cần xử lý.
					if err != nil {
						// Có thể log lỗi để quan sát
						log.Printf("[rabbitmq ingress %s] handler error (autoAck): %v", i.sourceName, err)
					}
					continue
				}
				if err != nil {
					_ = d.Nack(false /*multiple*/, i.requeueOnError)
				} else {
					_ = d.Ack(false /*multiple*/)
				}
			case <-ctx.Done():
				// Hủy consumer qua Cancel để đóng deliveries
				_ = i.ch.Cancel(i.consumerTag, false)
				return
			}
		}
	}()
	return nil
}

func (i *ingress) Stop(ctx context.Context) error {
	if i.stopped.Swap(true) {
		return nil
	}
	// Hủy consumer; nếu chưa cancel trong Start, Cancel tại đây
	_ = i.ch.Cancel(i.consumerTag, false)

	// Đợi goroutine dừng hoặc timeout theo ctx
	select {
	case <-i.doneCh:
	case <-ctx.Done():
	}
	// Đóng channel riêng của ingress
	closeCh := make(chan struct{})
	go func() {
		_ = i.ch.Close()
		close(closeCh)
	}()
	select {
	case <-closeCh:
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
	return nil
}
