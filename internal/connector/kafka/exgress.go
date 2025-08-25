package kafka

import (
	"context"
	"fmt"

	kafka "github.com/segmentio/kafka-go"
)

type kafkaEgress struct {
	cfg    EgressCfg
	parent *Connector
}

func (e *kafkaEgress) TargetName() string { return e.cfg.TargetName }

func (e *kafkaEgress) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	w := e.parent.writers[e.cfg.Topic]
	if w == nil {
		return fmt.Errorf("writer not found for topic %s", e.cfg.Topic)
	}

	key := []byte(meta["key"])
	return w.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: msg,
		// Headers: []kafka.Header{{Key:"...", Value:[]byte("...")}},
	})
}

func (e *kafkaEgress) Close() error { return nil }
