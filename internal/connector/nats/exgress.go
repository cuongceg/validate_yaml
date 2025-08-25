package nats

import (
	"context"
	"strings"

	"github.com/nats-io/nats.go"
)

type egress struct {
	owner *NATSConnector
	cfg   EgressConfig
}

func newEgress(owner *NATSConnector, cfg EgressConfig) *egress {
	return &egress{owner: owner, cfg: cfg}
}

func (e *egress) TargetName() string { return e.cfg.TargetName }

func (e *egress) Close() error { return nil }

func (e *egress) Publish(ctx context.Context, msg []byte, meta map[string]string) error {
	subj := e.cfg.Subject
	if e.cfg.JS.Enabled && e.cfg.JS.Subject != "" {
		subj = e.cfg.JS.Subject
	}
	if rk := meta["route_key"]; rk != "" {
		subj = strings.ReplaceAll(subj, "${route_key}", rk)
	}

	nmsg := &nats.Msg{Subject: subj, Data: msg}
	if len(e.cfg.Headers) > 0 || len(meta) > 0 {
		nmsg.Header = nats.Header{}
		for k, v := range e.cfg.Headers {
			nmsg.Header.Set(k, v)
		}
		for k, v := range meta { // allow meta to add/override selected headers
			if strings.HasPrefix(k, "hdr:") {
				nmsg.Header.Set(strings.TrimPrefix(k, "hdr:"), v)
			}
		}
		if e.cfg.JS.EnableDeDup {
			if id := meta["idempotencyKey"]; id != "" {
				nmsg.Header.Set(nats.MsgIdHdr, id)
			} else if rk := meta["route_key"]; rk != "" {
				nmsg.Header.Set(nats.MsgIdHdr, rk)
			}
		}
	}

	if e.cfg.JS.Enabled && e.owner.js != nil {
		_, err := e.owner.js.PublishMsg(nmsg, nats.Context(ctx))
		return err
	}
	return e.owner.nc.PublishMsg(nmsg)
}
