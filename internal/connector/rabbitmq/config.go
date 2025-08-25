package rabbitmq

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"
)

type RabbitMQConfig struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
	// Nếu cần TLS tùy biến (amqps), thiết lập tại đây (tùy chọn).
	TLS *TLSOptions `yaml:"tls,omitempty" json:"tls,omitempty"`
	// Danh sách ingress và egress (map sang core.Ingress/core.Egress).
	Ingresses []IngressConfig `yaml:"ingresses" json:"ingresses"`
	Egresses  []EgressConfig  `yaml:"egresses"  json:"egresses"`
}

// TLSOptions cho phép nạp CA/Cert/Key từ file & set InsecureSkipVerify nếu muốn.
type TLSOptions struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	RootCAPath         string `yaml:"rootCAPath,omitempty" json:"rootCAPath,omitempty"`
	ClientCertPath     string `yaml:"clientCertPath,omitempty" json:"clientCertPath,omitempty"`
	ClientKeyPath      string `yaml:"clientKeyPath,omitempty" json:"clientKeyPath,omitempty"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify,omitempty" json:"insecureSkipVerify,omitempty"`
}

// IngressConfig mô tả consumer đọc từ Queue trong RabbitMQ.
type IngressConfig struct {
	SourceName     string `yaml:"sourceName" json:"sourceName"` // bắt buộc: trả về từ SourceName()
	Queue          string `yaml:"queue" json:"queue"`           // bắt buộc
	ConsumerTag    string `yaml:"consumerTag,omitempty" json:"consumerTag,omitempty"`
	AutoAck        bool   `yaml:"autoAck,omitempty" json:"autoAck,omitempty"`
	Prefetch       int    `yaml:"prefetch,omitempty" json:"prefetch,omitempty"`
	PrefetchGlobal bool   `yaml:"prefetchGlobal,omitempty" json:"prefetchGlobal,omitempty"`
	RequeueOnError bool   `yaml:"requeueOnError,omitempty" json:"requeueOnError,omitempty"`
}

// EgressConfig mô tả publisher gửi vào Exchange/RoutingKey.
type EgressConfig struct {
	TargetName         string        `yaml:"targetName" json:"targetName"` // bắt buộc: trả về từ TargetName()
	Exchange           string        `yaml:"exchange" json:"exchange"`     // có thể rỗng nếu publish trực tiếp vào queue qua routingKey
	RoutingKey         string        `yaml:"routingKey,omitempty" json:"routingKey,omitempty"`
	Persistent         bool          `yaml:"persistent,omitempty" json:"persistent,omitempty"`
	DefaultContentType string        `yaml:"defaultContentType,omitempty" json:"defaultContentType,omitempty"`
	PublishTimeout     time.Duration `yaml:"publishTimeout,omitempty" json:"publishTimeout,omitempty"`
}

func buildTLSConfig(opts *TLSOptions) (*tls.Config, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil
	}
	cfg := &tls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if opts.RootCAPath != "" {
		caBytes, err := os.ReadFile(opts.RootCAPath)
		if err != nil {
			return nil, fmt.Errorf("read root CA: %w", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caBytes); !ok {
			return nil, fmt.Errorf("append root CA failed")
		}
		cfg.RootCAs = pool
	}
	if opts.ClientCertPath != "" && opts.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCertPath, opts.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
