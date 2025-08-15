package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type Direction string

const (
	Forward Direction = "forward"
	Reverse Direction = "reverse"
	Both    Direction = "both"
)

func (d *Direction) UnmarshalYAML(fn func(any) error) error {
	var s string
	if err := fn(&s); err != nil {
		return err
	}
	v := Direction(s)
	switch v {
	case Forward, Reverse, Both:
		*d = v
		return nil
	default:
		return fmt.Errorf("invalid direction=%q (valid: forward|reverse|both)", s)
	}
}

type Acks string

const (
	AtMostOnce  Acks = "at_most_once"
	AtLeastOnce Acks = "at_least_once"
)

func (a *Acks) UnmarshalYAML(fn func(any) error) error {
	var s string
	if err := fn(&s); err != nil {
		return err
	}
	v := Acks(s)
	switch v {
	case AtMostOnce, AtLeastOnce:
		*a = v
		return nil
	default:
		return fmt.Errorf("invalid qos.acks=%q (valid: at_most_once|at_least_once)", s)
	}
}

// Duration wraps time.Duration to parse YAML strings like "300ms", "5s"
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(fn func(any) error) error {
	var s string
	if err := fn(&s); err != nil {
		return err
	}
	dd, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dd
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}

type TLSBlock struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type QoS struct {
	Acks        Acks `yaml:"acks"`
	MaxInflight int  `yaml:"max_inflight"`
}

type TTL struct {
	Timeout Duration `yaml:"timeout"`
	MaxHops int      `yaml:"max_hops"`
}

type FilterConfig struct {
	Type   string   `yaml:"type"` // drop|minium
	Topics []string `yaml:"topics"`
	Window Duration `yaml:"window"` // chỉ dùng cho minimum
}

type Route struct {
	From      string         `yaml:"from"` // rabbit|kafka|nats
	To        []string       `yaml:"to"`
	Direction Direction      `yaml:"direction"`
	Filters   []FilterConfig `yaml:"filters"`
	QoS       QoS            `yaml:"qos"`
	TTL       TTL            `yaml:"ttl"`
}

type RabbitConfig struct {
	URL string   `yaml:"url"`
	TLS TLSBlock `yaml:"tls"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	TLS     TLSBlock `yaml:"tls"`
}

type NATSConfig struct {
	URL string   `yaml:"url"`
	TLS TLSBlock `yaml:"tls"`
}

type Persistence struct {
	Type string `yaml:"type"` // badger|bolt|none
	Path string `yaml:"path"`
}

type ProtobufConfig struct {
	Files   []string `yaml:"files"`
	MsgType string   `yaml:"msg_type"`
}

type AppConfig struct {
	LogLevel    string `yaml:"log_level"`
	MetricsAddr string `yaml:"metrics_addr"`
}

type Config struct {
	SchemaVersion int            `yaml:"schema_version"`
	App           AppConfig      `yaml:"app"`
	Routes        []Route        `yaml:"routes"`
	Rabbit        RabbitConfig   `yaml:"rabbit"`
	Kafka         KafkaConfig    `yaml:"kafka"`
	NATS          NATSConfig     `yaml:"nats"`
	Persistence   Persistence    `yaml:"persistence"`
	Protobuf      ProtobufConfig `yaml:"protobuf"`
	Compression   string         `yaml:"compression"` // none|snappy|zstd
}

// Helper to provide YAML KnownFields decoding:
func decodeYAMLStrict(data []byte, out any, strict bool) error {
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return err
	}
	// dec := yaml.Decoder{
	// 	KnownFields: strict,
	// }
	// yaml.v3 không có struct public để set KnownFields trên Decoder ctor,
	// nên dùng cách thủ công:
	d := yaml.NewDecoder(bytesReader(data))
	d.KnownFields(strict)
	return d.Decode(out)
}
