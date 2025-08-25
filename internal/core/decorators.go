package core

import (
	"context"
	"time"
)

type EgressWithLog struct{ Next Egress }

func (d EgressWithLog) TargetName() string { return d.Next.TargetName() }
func (d EgressWithLog) Close() error       { return d.Next.Close() }
func (d EgressWithLog) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	t0 := time.Now()
	err := d.Next.Publish(ctx, msg, meta)
	// TODO: log/metric/tracing…
	_ = t0
	return err
}
