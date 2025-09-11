package kafka

import (
	"fmt"
	"time"

	"github.com/cuongceg/validate_yaml/internal/core"
	kafka "github.com/segmentio/kafka-go"
)

type Connector struct {
	cfg     Config
	dialer  *kafka.Dialer
	readers map[string]*kafka.Reader // map[GroupID]*Reader
	writers map[string]*kafka.Writer
	ing     []core.Ingress
	eg      []core.Egress
}

func NewKafkaConnector(raw any) (core.Connector, error) {
	cfg := raw.(Config)
	c := &Connector{
		cfg: cfg,
	}
	c.readers = make(map[string]*kafka.Reader, len(cfg.Ingresses))
	c.writers = make(map[string]*kafka.Writer, len(cfg.Egresses))
	for _, ic := range cfg.Ingresses {
		c.ing = append(c.ing, &kafkaIngress{cfg: ic, parent: c})
	}
	for _, ec := range cfg.Egresses {
		c.eg = append(c.eg, &kafkaEgress{cfg: ec, parent: c})
	}
	return c, nil
}

func (c *Connector) Open() error {
	// Dialer dùng chung: TLS/SASL, timeouts
	c.dialer = &kafka.Dialer{
		Timeout:  10 * time.Second,
		ClientID: c.cfg.ClientID,
		//TODO: TLS & SASL config
	}

	// Tạo Readers theo ingress (Group consumer)
	for _, ic := range c.cfg.Ingresses {
		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:         c.cfg.Brokers,
			GroupID:         ic.GroupID,
			Topic:           ic.Topic,
			Dialer:          c.dialer,
			MinBytes:        1 << 10,
			MaxBytes:        10 << 20,
			MaxWait:         5 * time.Millisecond,
			QueueCapacity:   100000,
			ReadLagInterval: 0,
			CommitInterval:  0,
		})
		c.readers[ic.SourceName] = r
	}

	for _, ec := range c.cfg.Egresses {
		if _, ok := c.writers[ec.Topic]; !ok {
			c.writers[ec.Topic] = &kafka.Writer{
				Addr:                   kafka.TCP(c.cfg.Brokers...),
				Topic:                  ec.Topic,
				Balancer:               &kafka.Hash{}, // hoặc LeastBytes, RoundRobin
				AllowAutoTopicCreation: false,
				BatchTimeout:           10 * time.Millisecond,
				BatchBytes:             128 << 10,
			}
		}
	}
	return nil
}

func (c *Connector) Close() error {
	var err error
	for _, r := range c.readers {
		err = r.Close()
	}
	for _, w := range c.writers {
		err = w.Close()
	}
	fmt.Printf("Kafka %q connector closed\n", c.cfg.Name)
	return err
}

func (c *Connector) Name() string {
	return c.cfg.Name
}
func (c *Connector) Ingresses() []core.Ingress { return c.ing }
func (c *Connector) Egresses() []core.Egress   { return c.eg }
