// internal/router/adapters.go
package router

import (
	"context"
	"fmt"
	"time"

	core "github.com/cuongceg/validate_yaml/internal/core"
)

type busAdapter struct {
	name      string
	conn      core.Connector
	ingByName map[string]core.Ingress
	egByName  map[string]core.Egress
}

func NewBusFromConnector(c core.Connector) (Bus, error) {
	ing := make(map[string]core.Ingress)
	for _, x := range c.Ingresses() {
		ing[x.SourceName()] = x
	}
	eg := make(map[string]core.Egress)
	for _, x := range c.Egresses() {
		eg[x.TargetName()] = x
	}
	return &busAdapter{
		name:      c.Name(),
		conn:      c,
		ingByName: ing,
		egByName:  eg,
	}, nil
}

func (b *busAdapter) Subscribe(ctx context.Context, sourceName string, h func(context.Context, *Message) error) error {
	ing, ok := b.ingByName[sourceName]
	if !ok {
		return fmt.Errorf("bus[%s]: ingress %q not found", b.name, sourceName)
	}
	// core.Handler: (ctx, msg []byte, meta map[string]string) error
	return ing.Start(ctx, func(c context.Context, payload []byte, meta map[string]string) error {
		// Chuyển sang router.Message
		msg := &Message{
			Value: payload,        // giữ nguyên bytes
			Meta:  toAnyMap(meta), // map[string]string -> map[string]any
		}
		return h(c, msg)
	})
}

func (b *busAdapter) Publish(ctx context.Context, targetName string, msg *Message) error {
	eg, ok := b.egByName[targetName]
	if !ok {
		return fmt.Errorf("bus[%s]: egress %q not found", b.name, targetName)
	}
	return eg.Publish(ctx, msg.Value, toStringMap(msg.Meta))
}

func (b *busAdapter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for name, ing := range b.ingByName {
		if err := ing.Stop(ctx); err != nil {
			// ghi log tùy ý; ở đây trả lỗi tổng hợp đơn giản
			return fmt.Errorf("bus[%s]: stop ingress %q: %w", b.name, name, err)
		}
	}
	for name, eg := range b.egByName {
		if err := eg.Close(); err != nil {
			return fmt.Errorf("bus[%s]: close egress %q: %w", b.name, name, err)
		}
	}
	return b.conn.Close()
}

// BuildBuses: nhận một tập connector và tạo map Bus theo Name()
func BuildBusesFromConnectors(connectors []core.Connector) (map[string]Bus, error) {
	out := make(map[string]Bus, len(connectors))
	for _, c := range connectors {
		b, err := NewBusFromConnector(c)
		if err != nil {
			return nil, err
		}
		out[c.Name()] = b
	}
	return out, nil
}

func toAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toStringMap(m map[string]any) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			out[k] = vv
		case fmt.Stringer:
			out[k] = vv.String()
		default:
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}
