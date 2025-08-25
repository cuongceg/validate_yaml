package nats

import (
	"fmt"

	"github.com/cuongceg/validate_yaml/internal/core"
)

func factory(cfg any) (core.Connector, error) {
	var c ConnectorConfig
	switch v := cfg.(type) {
	case ConnectorConfig:
		c = v
	case *ConnectorConfig:
		if v == nil {
			return nil, fmt.Errorf("nil *ConnectorConfig")
		}
		c = *v
	default:
		return nil, fmt.Errorf("invalid cfg type: %T; expected natsconn.ConnectorConfig", cfg)
	}
	return NewConnector(c), nil
}

func init() { core.RegisterConnector("nats", factory) }
