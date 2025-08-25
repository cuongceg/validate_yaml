package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"

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

	go func() {
		for {
			m, err := r.FetchMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// log & tiếp tục
				continue
			}
			meta := map[string]string{
				"topic":     m.Topic,
				"partition": strconv.Itoa(m.Partition),
				"offset":    strconv.FormatInt(m.Offset, 10),
				"key":       string(m.Key),
			}
			if err := h(ctx, m.Value, meta); err == nil {
				_ = r.CommitMessages(ctx, m) // commit offset
			} else {
				// tuỳ chính sách: không commit để retry
			}
		}
	}()
	return nil
}

func (i *kafkaIngress) Stop(ctx context.Context) error { return nil }
