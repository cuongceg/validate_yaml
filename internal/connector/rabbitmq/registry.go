package rabbitmq

import (
	"fmt"

	"github.com/cuongceg/validate_yaml/internal/core"
)

func factory(cfg any) (core.Connector, error) {
	switch c := cfg.(type) {
	case RabbitMQConfig:
		return NewConnector(c), nil
	case *RabbitMQConfig:
		if c == nil {
			return nil, fmt.Errorf("nil *RabbitMQConfig")
		}
		return NewConnector(*c), nil
	default:
		return nil, fmt.Errorf("unexpected config type %T for rabbitmq", cfg)
	}
}

func init() {
	core.RegisterConnector("rabbitmq", factory)
}
