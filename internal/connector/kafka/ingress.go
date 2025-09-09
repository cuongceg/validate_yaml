package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cuongceg/validate_yaml/internal/core"
	kafka "github.com/segmentio/kafka-go"
)

type kafkaIngress struct {
	cfg    IngressCfg
	parent *Connector
	// có thể giữ con trỏ reader nếu bạn map theo SourceName
}

func (i *kafkaIngress) SourceName() string { return i.cfg.SourceName }

func (i *kafkaIngress) Start(ctx context.Context, h core.Handler) error {
	var r *kafka.Reader
	for _, rr := range i.parent.readers {
		if rr.Config().Topic == i.cfg.Topic {
			r = rr
			break
		}
	}
	if r == nil {
		return fmt.Errorf("reader not found for topic %s", i.cfg.Topic)
	}
	wg := &sync.WaitGroup{}
	wg.Add(10)
	for j := 0; j < 10; j++ {
		go func() {
			defer wg.Done()
			for {
				m, err := r.FetchMessage(ctx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					// log & tiếp tục
					continue
				}
				meta := map[string]string{}
				for _, v := range m.Headers {
					meta[v.Key] = string(v.Value)
				}
				// Handler sẽ CHỈ trả nil sau khi publish RabbitMQ OK (router đảm nhiệm)
				if err := h(ctx, m.Value, meta); err == nil {
					_ = r.CommitMessages(ctx, m) // commit offset sau khi downstream OK
				} else {
					// tùy chính sách: không commit để retry
				}
			}
		}()
	}
	return nil
}

func (i *kafkaIngress) Stop(ctx context.Context) error { return nil }
