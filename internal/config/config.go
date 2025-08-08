package config

import (
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Connectors ConnectorsConfig `yaml:"connectors"`
	Filters    []FilterConfig   `yaml:"filters"`
}

type AppConfig struct {
	LogLevel    string `yaml:"log_level"`
	MetricsAddr string `yaml:"metrics_addr"`
	DryRun      bool   `yaml:"dry_run"`
}

type ConnectorsConfig struct {
	RabbitMQ *RabbitMQConfig `yaml:"rabbitmq"`
	Kafka    *KafkaConfig    `yaml:"kafka"`
	NATS     *NATSConfig     `yaml:"nats"`
}

type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type RabbitMQConfig struct {
	Enabled  bool      `yaml:"enabled"`
	URL      string    `yaml:"url"`
	Queue    string    `yaml:"queue"`
	Prefetch int       `yaml:"prefetch"`
	TTL      string    `yaml:"ttl"`
	TLS      TLSConfig `yaml:"tls"`
}

type KafkaConfig struct {
	Enabled  bool      `yaml:"enabled"`
	Brokers  []string  `yaml:"brokers"`
	Topic    string    `yaml:"topic"`
	Group    string    `yaml:"group"`
	Acks     string    `yaml:"acks"`
	LingerMS int       `yaml:"linger_ms"`
	TLS      TLSConfig `yaml:"tls"`
}

type NATSConfig struct {
	Enabled   bool      `yaml:"enabled"`
	URL       string    `yaml:"url"`
	Stream    string    `yaml:"stream"`
	Subject   string    `yaml:"subject"`
	PullBatch int       `yaml:"pull_batch"`
	TLS       TLSConfig `yaml:"tls"`
}

type FilterConfig struct {
	Name  string      `yaml:"name"`
	Type  string      `yaml:"type"`
	Match MatchConfig `yaml:"match"`
}

type MatchConfig struct {
	Field string      `yaml:"field"`
	Op    string      `yaml:"op"`
	Value interface{} `yaml:"value"`
}

func Load(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	applyDefaults(&cfg)
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(c *Config) {
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.Connectors.RabbitMQ != nil {
		if c.Connectors.RabbitMQ.Prefetch == 0 {
			c.Connectors.RabbitMQ.Prefetch = 200
		}
		if c.Connectors.RabbitMQ.TTL == "" {
			c.Connectors.RabbitMQ.TTL = "60s"
		}
	}
	if c.Connectors.Kafka != nil {
		if c.Connectors.Kafka.Acks == "" {
			c.Connectors.Kafka.Acks = "all"
		}
	}
	if c.Connectors.NATS != nil {
		if c.Connectors.NATS.PullBatch == 0 {
			c.Connectors.NATS.PullBatch = 64
		}
	}
}

// Helper to parse duration during validation or runtime use.
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, errors.New("empty duration")
	}
	return time.ParseDuration(s)
}
