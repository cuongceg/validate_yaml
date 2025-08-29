package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	core "github.com/cuongceg/validate_yaml/internal/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

type egress struct {
	targetName         string
	exchange           string
	routingKeyTpl      string
	persistent         bool
	defaultContentType string
	publishTimeout     time.Duration

	conn *amqp.Connection
	ch   *amqp.Channel

	confirms <-chan amqp.Confirmation
	returns  <-chan amqp.Return

	mu sync.Mutex // bảo vệ Publish tuần tự trên channel
}

func newEgress(conn *amqp.Connection, cfg EgressConfig) (core.Egress, error) {
	if cfg.TargetName == "" {
		return nil, fmt.Errorf("egress requires targetName")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("egress channel: %w", err)
	}

	// Bật publisher confirms
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("enable confirms: %w", err)
	}

	// Đăng ký kênh nhận Return/Confirm
	rets := ch.NotifyReturn(make(chan amqp.Return, 1))
	confs := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	return &egress{
		targetName:         cfg.TargetName,
		exchange:           cfg.Exchange,
		routingKeyTpl:      cfg.RoutingKey,
		persistent:         cfg.Persistent,
		defaultContentType: cfg.DefaultContentType,
		publishTimeout:     cfg.PublishTimeout,
		conn:               conn,
		ch:                 ch,
		confirms:           confs,
		returns:            rets,
	}, nil
}

func (e *egress) TargetName() string { return e.targetName }

func (e *egress) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// deadline
	var cancel func()
	if _, has := ctx.Deadline(); !has && e.publishTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, e.publishTimeout)
		defer cancel()
	}

	// content-type
	ct := e.defaultContentType
	if v := meta["content-type"]; v != "" {
		ct = v
	}

	// headers
	hdrs := amqp.Table{}
	for k, v := range meta {
		if k == "content-type" {
			continue
		}
		hdrs[k] = v
	}

	// render routing key từ template (nếu có), else dùng nguyên cfg
	rk := e.routingKeyTpl
	// if e.routingKeyTpl != nil {
	// 	var b bytes.Buffer
	// 	// meta là map[string]string → truyền luôn
	// 	if err := e.routingKeyTpl.Execute(&b, meta); err != nil {
	// 		return fmt.Errorf("render routing key: %w", err)
	// 	}
	// 	rk = b.String()
	// }

	pub := amqp.Publishing{
		ContentType: ct,
		Body:        msg,
		Headers:     hdrs,
		DeliveryMode: func() uint8 {
			if e.persistent {
				return amqp.Persistent
			}
			return amqp.Transient
		}(),
		// bạn có thể set CorrelationId/ReplyTo nếu cần end-to-end ack
	}

	// mandatory=true để nhận Return nếu unroutable
	if err := e.ch.PublishWithContext(ctx, e.exchange, rk, true, false, pub); err != nil {
		fmt.Printf("PublishWithContext with exchange: %s and routing key %s\n", e.exchange, rk)
		return err
	}

	// Đợi kết quả: Return hoặc Confirm
	// Lưu ý: với kiểu publish tuần tự (nhờ e.mu), confirm/return tiếp theo được coi là của message vừa publish.
	select {
	case ret := <-e.returns:
		// Unroutable: không khớp bất kỳ binding nào
		return fmt.Errorf("unroutable: exchange=%s rk=%s reply=%d %s", ret.Exchange, ret.RoutingKey, ret.ReplyCode, ret.ReplyText)
	case conf := <-e.confirms:
		if !conf.Ack {
			return fmt.Errorf("broker NACKed: exchange=%s rk=%s", e.exchange, rk)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("publish confirm timeout: exchange=%s rk=%s", e.exchange, rk)
	}
}

func (e *egress) Close() error {
	return e.ch.Close()
}
