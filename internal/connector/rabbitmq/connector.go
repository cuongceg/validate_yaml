package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	core "github.com/cuongceg/validate_yaml/internal/core"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Connector struct {
	cfg RabbitMQConfig

	conn *amqp.Connection
	// Kênh dùng để khai báo & chia sẻ cho ingress/egress; mỗi ingress sẽ tạo consumer riêng.
	ch *amqp.Channel

	ing []core.Ingress
	eg  []core.Egress

	mu     sync.RWMutex
	opened bool
}

func NewConnector(cfg RabbitMQConfig) *Connector {
	return &Connector{cfg: cfg}
}

func (c *Connector) Name() string {
	return c.cfg.Name
}

func (c *Connector) Open() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opened {
		return nil
	}

	var dialCfg amqp.Config
	// Thiết lập TLS nếu cần
	if c.cfg.TLS != nil && c.cfg.TLS.Enabled {
		tlsCfg, err := buildTLSConfig(c.cfg.TLS)
		if err != nil {
			return err
		}
		dialCfg.TLSClientConfig = tlsCfg
	}

	conn, err := amqp.DialConfig(c.cfg.URL, dialCfg)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create channel: %w", err)
	}

	c.conn = conn
	c.ch = ch

	// Khởi tạo danh sách ingresses / egresses từ config
	ingresses := make([]core.Ingress, 0, len(c.cfg.Ingresses))
	for _, ic := range c.cfg.Ingresses {
		ing, err := newIngress(c.conn, ic)
		if err != nil {
			_ = c.Close()
			return err
		}
		ingresses = append(ingresses, ing)
	}
	egresses := make([]core.Egress, 0, len(c.cfg.Egresses))
	for _, ec := range c.cfg.Egresses {
		eg, err := newEgress(c.conn, ec)
		if err != nil {
			_ = c.Close()
			return err
		}
		egresses = append(egresses, eg)
	}

	c.ing = ingresses
	c.eg = egresses
	c.opened = true
	return nil
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error

	// Đóng egress trước
	for _, e := range c.eg {
		if err := e.Close(); err != nil && firstErr == nil {
			firstErr = err
			fmt.Printf("Error closing egress: %v\n", err)
		}
	}
	c.eg = nil
	// Dừng/đóng ingress
	for _, i := range c.ing {
		err := i.Stop(context.Background())
		if err != nil && firstErr == nil {
			firstErr = err
			fmt.Printf("Error stopping ingress: %v\n", err)
		}
	}
	c.ing = nil
	if c.ch != nil {
		if err := c.ch.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.ch = nil
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.conn = nil
	}
	c.opened = false
	fmt.Printf("RabbitMQ %q connector closed\n", c.cfg.Name)
	return firstErr
}

func (c *Connector) Ingresses() []core.Ingress {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]core.Ingress(nil), c.ing...)
}

func (c *Connector) Egresses() []core.Egress {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]core.Egress(nil), c.eg...)
}
