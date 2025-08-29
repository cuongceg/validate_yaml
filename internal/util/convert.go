package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	config "github.com/cuongceg/validate_yaml/internal/config"
	kafka "github.com/cuongceg/validate_yaml/internal/connector/kafka"
	nats "github.com/cuongceg/validate_yaml/internal/connector/nats"
	rabbitmq "github.com/cuongceg/validate_yaml/internal/connector/rabbitmq"
	core "github.com/cuongceg/validate_yaml/internal/core"
)

func decodeParams[T any](m map[string]interface{}) (T, error) {
	var zero T
	b, err := json.Marshal(m)
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(b, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func MapConnectors(ctx context.Context, uc *config.UserConfig) (map[string]core.Connector, error) {
	conns := make(map[string]core.Connector, len(uc.Connectors))

	for _, c := range uc.Connectors {
		switch c.Type {
		case "rabbitmq":
			// 1) map UserConfig -> rabbitmq.RabbitMQConfig
			rmqCfg, err := decodeParams[rabbitmq.RabbitMQConfig](c.Params)
			if err != nil {
				return nil, fmt.Errorf("connector %q: decode params: %w", c.Name, err)
			}
			rmqCfg.Name = c.Name
			// Map ingress
			rmqCfg.Ingresses = nil
			for _, ig := range c.Ingress {
				rmqCfg.Ingresses = append(rmqCfg.Ingresses, rabbitmq.IngressConfig{
					SourceName:     ig.SourceName,
					Queue:          ig.Queue,
					AutoAck:        false,
					Prefetch:       200,
					PrefetchGlobal: false,
					RequeueOnError: true,
				})
			}
			// Map egress
			rmqCfg.Egresses = nil
			for _, eg := range c.Egress {
				rmqCfg.Egresses = append(rmqCfg.Egresses, rabbitmq.EgressConfig{
					TargetName:         eg.Name,
					Exchange:           eg.Exchange,
					RoutingKey:         eg.RoutingKeyTemplate, // nếu là template, bạn có thể render ở tầng route
					Persistent:         true,
					DefaultContentType: "application/octet-stream",
					PublishTimeout:     5 * time.Second,
				})
			}
			if c.TLS != nil && c.TLS.Enabled {
				rmqCfg.TLS = &rabbitmq.TLSOptions{
					Enabled:            true,
					RootCAPath:         c.TLS.CAFile,
					ClientCertPath:     c.TLS.CertFile,
					ClientKeyPath:      c.TLS.KeyFile,
					InsecureSkipVerify: c.TLS.InsecureSkipVerify,
				}
			}

			// 2) tạo connector qua registry
			conn, err := core.BuildConnector("rabbitmq", rmqCfg)
			if err != nil {
				return nil, fmt.Errorf("connector %q: build: %w", c.Name, err)
			}

			if err := conn.Open(ctx); err != nil {
				return nil, fmt.Errorf("connector %q: open: %w", c.Name, err)
			}
			conns[c.Name] = conn

		case "kafka":
			kafkaCfg, err := decodeParams[kafka.Config](c.Params)
			if err != nil {
				return nil, fmt.Errorf("connector %q: decode params: %w", c.Name, err)
			}
			kafkaCfg.Name = c.Name
			kafkaCfg.Ingresses = nil
			for _, ig := range c.Ingress {
				kafkaCfg.Ingresses = append(kafkaCfg.Ingresses, kafka.IngressCfg{
					SourceName: ig.SourceName,
					Topic:      ig.Topic,
					GroupID:    ig.GroupID,
				})
			}

			kafkaCfg.Egresses = nil
			for _, eg := range c.Egress {
				kafkaCfg.Egresses = append(kafkaCfg.Egresses, kafka.EgressCfg{
					TargetName: eg.Name,
					Topic:      eg.TopicTemplate,
					//KeyFrom:    eg.KeyFrom,
				})
			}
			// if c.SASL != nil && c.SASL.Enabled {
			// 	kafkaCfg.SASL = &kafka.SASL{
			// 		Enable:    true,
			// 		Mechanism: c.SASL.Mechanism,
			// 		Username:  c.SASL.Username,
			// 		Password:  c.SASL.Password,
			// 	}
			// }
			if c.TLS != nil && c.TLS.Enabled {
				kafkaCfg.TLS = &kafka.TLS{
					Enable:   true,
					Insecure: c.TLS.InsecureSkipVerify,
					CAFile:   c.TLS.CAFile,
					CertFile: c.TLS.CertFile,
					KeyFile:  c.TLS.KeyFile,
				}
			}

			conn, err := core.BuildConnector("kafka", kafkaCfg)
			if err != nil {
				return nil, fmt.Errorf("connector %q: build: %w", c.Name, err)
			}
			if err := conn.Open(ctx); err != nil {
				return nil, fmt.Errorf("connector %q: open: %w", c.Name, err)
			}
			conns[c.Name] = conn

		case "nats":
			_, err := decodeParams[nats.ConnectorConfig](c.Params)
			if err != nil {
				return nil, fmt.Errorf("connector %q: decode params: %w", c.Name, err)
			}
			// natsCfg.Name = c.Name
			// natsCfg.Ingresses = nil
			// for _, ig := range c.Ingress {
			// 	natsCfg.Ingresses = append(natsCfg.Ingresses, nats.IngressConfig{
			// 		SourceName:    ig.SourceName,
			// 		Subjects:      []string{ig.Subject},
			// 		HeadersToMeta: ig.HeadersToMeta,
			// 		KeyFrom:       ig.KeyFrom,
			// 		JS: nats.IngressJetStream{
			// 			Enabled:       ig.JetStream.Enabled,
			// 			Subject:       ig.JetStream.Subject,
			// 			Durable:       ig.JetStream.Durable,
			// 		},
			// 	})
			// }

		default:
			return nil, fmt.Errorf("connector %q: unsupported type %q", c.Name, c.Type)
		}
	}
	return conns, nil
}
