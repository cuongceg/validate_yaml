package nats

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/cuongceg/validate_yaml/internal/core"
	"github.com/nats-io/nats.go"
	"github.com/tidwall/gjson"
)

type ingress struct {
	owner *NATSConnector
	cfg   IngressConfig
	// runtime
	subs []*nats.Subscription
}

func newIngress(owner *NATSConnector, cfg IngressConfig) *ingress {
	return &ingress{owner: owner, cfg: cfg}
}

func (i *ingress) SourceName() string { return i.cfg.SourceName }

func (i *ingress) Start(ctx context.Context, h core.Handler) error {
	if h == nil {
		return errors.New("handler required")
	}
	if i.cfg.JS.Enabled {
		return i.startJS(ctx, h)
	}
	if len(i.cfg.Subjects) == 0 {
		return errors.New("ingress.subjects empty for core NATS")
	}
	for _, subj := range i.cfg.Subjects {
		sub, err := i.owner.nc.QueueSubscribe(subj, i.cfg.JS.Queue, func(m *nats.Msg) {
			i.handle(ctx, m, h, false)
		})
		if err != nil {
			return err
		}
		i.subs = append(i.subs, sub)
	}
	return i.owner.nc.Flush()
}

func (i *ingress) startJS(ctx context.Context, h core.Handler) error {
	js := i.owner.js
	cfg := i.cfg.JS
	if cfg.Subject == "" {
		return errors.New("ingress.jetstream.subject required")
	}

	if cfg.PullMode {
		sub, err := js.PullSubscribe(cfg.Subject, cfg.Durable)
		if err != nil {
			return err
		}
		i.subs = append(i.subs, sub)
		batch := cfg.PullBatch
		if batch <= 0 {
			batch = 32
		}
		interval := cfg.PullInterval
		if interval <= 0 {
			interval = 200 * time.Millisecond
		}
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					msgs, err := sub.Fetch(batch, nats.Context(ctx))
					if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, nats.ErrTimeout) {
						log.Printf("[NATS][JS] fetch: %v", err)
						continue
					}
					for _, m := range msgs {
						i.handle(ctx, m, h, true)
					}
				}
			}
		}()
		return nil
	}
	sub, err := js.QueueSubscribe(cfg.Subject, cfg.Queue, func(m *nats.Msg) { i.handle(ctx, m, h, true) }, nats.Durable(cfg.Durable), nats.ManualAck(), nats.AckWait(cfg.AckWait), nats.MaxAckPending(cfg.MaxAckPending))
	if err != nil {
		return err
	}
	i.subs = append(i.subs, sub)
	return nil
}

func (i *ingress) Stop(ctx context.Context) error {
	for _, s := range i.subs {
		_ = s.Drain()
	}
	i.subs = nil
	return nil
}

func (i *ingress) handle(ctx context.Context, m *nats.Msg, h core.Handler, isJS bool) {
	meta := map[string]string{
		"subject": m.Subject,
	}
	// headers → meta (selected)
	if m.Header != nil {
		for _, k := range i.cfg.HeadersToMeta {
			if v := m.Header.Get(k); v != "" {
				meta[k] = v
			}
		}
		if id := m.Header.Get(nats.MsgIdHdr); id != "" {
			meta["idempotencyKey"] = id
		}
	}
	// KeyFrom
	if kf := i.cfg.KeyFrom; kf != "" {
		if strings.HasPrefix(kf, "payload.") {
			path := strings.TrimPrefix(kf, "payload.")
			val := gjson.GetBytes(m.Data, path)
			if val.Exists() {
				meta["route_key"] = val.String()
			}
		} else if strings.HasPrefix(kf, "header.") {
			name := strings.TrimPrefix(kf, "header.")
			if v := m.Header.Get(name); v != "" {
				meta["route_key"] = v
			}
		}
	}

	if err := h(ctx, m.Data, meta); err != nil {
		log.Printf("[NATS] handler error: %v", err)
		if isJS {
			_ = m.Nak()
		}
		return
	}
	if isJS {
		_ = m.Ack()
	}
}
