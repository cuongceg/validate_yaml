package core

import "context"

type Handler func(ctx context.Context, msg []byte, meta map[string]string) error

type Connector interface {
	Name() string
	Open() error
	Close() error
	Ingresses() []Ingress
	Egresses() []Egress
}

type Ingress interface {
	SourceName() string
	Start(ctx context.Context, h Handler) error
	Stop(ctx context.Context) error
}

type Egress interface {
	TargetName() string
	Publish(ctx context.Context, msg []byte, meta map[string]string) error
	Close() error
}
