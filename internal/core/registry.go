package core

import "fmt"

type FactoryFunc func(cfg any) (Connector, error)

var connectors = map[string]FactoryFunc{}

func RegisterConnector(kind string, f FactoryFunc) {
	connectors[kind] = f
}

func BuildConnector(kind string, cfg any) (Connector, error) {
	f, ok := connectors[kind]
	if !ok {
		return nil, fmt.Errorf("unknown connector type: %s", kind)
	}
	return f(cfg)
}
