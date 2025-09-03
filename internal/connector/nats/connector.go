package nats

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cuongceg/validate_yaml/internal/core"
	"github.com/nats-io/nats.go"
)

type NATSConnector struct {
	cfg ConnectorConfig
	nc  *nats.Conn
	js  nats.JetStreamContext

	ing []core.Ingress
	eg  []core.Egress

	mu sync.RWMutex
}

func NewConnector(cfg ConnectorConfig) *NATSConnector {
	return &NATSConnector{cfg: cfg}
}

func (c *NATSConnector) Name() string { return c.cfg.Name }

func (c *NATSConnector) Open() error {
	if len(c.cfg.Servers) == 0 {
		return errors.New("nats servers not configured")
	}
	opts := []nats.Option{
		nats.Name(c.cfg.ClientName),
		nats.MaxReconnects(c.cfg.MaxReconnects),
		nats.ReconnectWait(c.cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) { log.Printf("[NATS] disconnected: %v", err) }),
		nats.ReconnectHandler(func(nc *nats.Conn) { log.Printf("[NATS] reconnected: %s", nc.ConnectedUrl()) }),
		nats.ClosedHandler(func(nc *nats.Conn) { log.Printf("[NATS] closed: %v", nc.LastError()) }),
	}
	if tlsCfg := c.cfg.TLS.ToTLSConfig(); tlsCfg != nil {
		opts = append(opts, nats.Secure(tlsCfg))
	}
	// auth chain
	if c.cfg.Auth.Token != "" {
		opts = append(opts, nats.Token(c.cfg.Auth.Token))
	} else if c.cfg.Auth.Username != "" || c.cfg.Auth.Password != "" {
		opts = append(opts, nats.UserInfo(c.cfg.Auth.Username, c.cfg.Auth.Password))
	} else if c.cfg.Auth.NKeySeed != "" || c.cfg.Auth.NKeySeedFile != "" {
		kp, err := deriveNKeyPair(c.cfg.Auth)
		if err != nil {
			return err
		}
		opts = append(opts, nats.Nkey(kp.pub, func(nonce []byte) ([]byte, error) { return ed25519.Sign(kp.priv, nonce), nil }))
	}

	url := strings.Join(c.cfg.Servers, ",")
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	c.nc = nc

	// JetStream if any ingress/egress needs it
	needJS := false
	for _, ig := range c.cfg.Ingresses {
		if ig.JS.Enabled {
			needJS = true
			break
		}
	}
	if !needJS {
		for _, eg := range c.cfg.Egresses {
			if eg.JS.Enabled {
				needJS = true
				break
			}
		}
	}
	if needJS {
		js, err := nc.JetStream()
		if err != nil {
			return fmt.Errorf("jetstream: %w", err)
		}
		c.js = js
	}

	// materialize ingress/egress instances
	c.ing = make([]core.Ingress, 0, len(c.cfg.Ingresses))
	for _, ig := range c.cfg.Ingresses {
		c.ing = append(c.ing, newIngress(c, ig))
	}
	c.eg = make([]core.Egress, 0, len(c.cfg.Egresses))
	for _, eg := range c.cfg.Egresses {
		c.eg = append(c.eg, newEgress(c, eg))
	}
	return nil
}

func (c *NATSConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.eg {
		_ = e.Close()
	}
	for _, i := range c.ing {
		_ = i.Stop(context.Background())
	}
	if c.nc != nil {
		c.nc.Close()
	}
	fmt.Printf("NATS %q connector closed\n", c.cfg.Name)
	return nil
}

func (c *NATSConnector) Ingresses() []core.Ingress { return c.ing }
func (c *NATSConnector) Egresses() []core.Egress   { return c.eg }

type nkeyPair struct {
	pub  string
	priv ed25519.PrivateKey
}

func deriveNKeyPair(a AuthConfig) (nkeyPair, error) {
	if a.NKeySeed == "" {
		return nkeyPair{}, errors.New("nkey seed not provided (seed/seedFile)")
	}
	// In production, parse seed using nkeys lib. Here we just allocate a key-size buffer.
	return nkeyPair{pub: "", priv: make(ed25519.PrivateKey, ed25519.PrivateKeySize)}, nil
}
