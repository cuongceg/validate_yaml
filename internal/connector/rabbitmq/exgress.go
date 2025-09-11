package rabbitmq

import (
	"context"
	"fmt"

	//"sync"
	"sync/atomic"
	"time"

	core "github.com/cuongceg/validate_yaml/internal/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Một channel AMQP cho 1 lane publish
type pubLane struct {
	ch       *amqp.Channel
	confirms <-chan amqp.Confirmation
	//mu       sync.Mutex // tuần tự hóa publish trong lane để mapping confirm đúng msg
}

type egress struct {
	targetName         string
	exchange           string
	routingKeyTpl      string
	persistent         bool
	defaultContentType string
	publishTimeout     time.Duration

	conn *amqp.Connection

	lanes     []pubLane
	rrCounter uint64 // round-robin chọn lane
}

func newEgress(conn *amqp.Connection, cfg EgressConfig) (core.Egress, error) {
	if cfg.TargetName == "" {
		return nil, fmt.Errorf("egress requires targetName")
	}

	poolSize := 16

	lanes := make([]pubLane, 0, poolSize)
	for i := 0; i < poolSize; i++ {
		ch, err := conn.Channel()
		if err != nil {
			return nil, fmt.Errorf("egress channel: %w", err)
		}
		if err := ch.Confirm(false); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("enable confirms: %w", err)
		}
		confs := ch.NotifyPublish(make(chan amqp.Confirmation, 2048))
		lanes = append(lanes, pubLane{
			ch:       ch,
			confirms: confs,
		})
	}

	return &egress{
		targetName:         cfg.TargetName,
		exchange:           cfg.Exchange,
		routingKeyTpl:      cfg.RoutingKey,
		persistent:         cfg.Persistent,
		defaultContentType: cfg.DefaultContentType,
		publishTimeout:     cfg.PublishTimeout,
		conn:               conn,
		lanes:              lanes,
	}, nil
}

func (e *egress) TargetName() string { return e.targetName }

func (e *egress) pickLane() *pubLane {
	idx := int(atomic.AddUint64(&e.rrCounter, 1) % uint64(len(e.lanes)))
	return &e.lanes[idx]
}

func (e *egress) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	ln := e.pickLane()
	// ln.mu.Lock() // giữ FIFO trong lane
	// defer ln.mu.Unlock()

	// deadline
	var cancel func()
	if _, has := ctx.Deadline(); !has && e.publishTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, e.publishTimeout)
		defer cancel()
	}

	hdrs := amqp.Table{}
	for k, v := range meta {
		hdrs[k] = v
	}

	// routing key (tạm: dùng template string sẵn)
	rk := e.routingKeyTpl

	pub := amqp.Publishing{
		ContentType:  "application/x-protobuf",
		Body:         msg,
		Headers:      hdrs,
		DeliveryMode: amqp.Transient,
		// DeliveryMode: func() uint8 {
		// 	if e.persistent {
		// 		return amqp.Persistent
		// 	}
		// 	return amqp.Transient
		// }(),
	}

	if err := ln.ch.PublishWithContext(ctx, e.exchange, rk /*mandatory*/, false, false, pub); err != nil {
		fmt.Printf("PublishWithContext with exchange: %s and routing key %s with errors %s\n", e.exchange, rk, err)
		return err
	}
	//Chờ confirm/return của CHÍNH message vừa publish (trong lane này)
	select {
	// case ret := <-ln.returns:
	// 	return fmt.Errorf("unroutable: exchange=%s rk=%s reply=%d %s", ret.Exchange, ret.RoutingKey, ret.ReplyCode, ret.ReplyText)
	case conf := <-ln.confirms:
		if !conf.Ack {
			return fmt.Errorf("broker NACKed: exchange=%s rk=%s", e.exchange, rk)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		return fmt.Errorf("publish confirm timeout: exchange=%s rk=%s", e.exchange, rk)
	}
}

func (e *egress) Close() error {
	var firstErr error
	for i := range e.lanes {
		if err := e.lanes[i].ch.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
