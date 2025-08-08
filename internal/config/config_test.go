package config

import (
	"strings"
	"testing"
)

const validYAML = `
app:
  log_level: info
connectors:
  rabbitmq:
    enabled: true
    url: "amqp://user:pass@localhost:5672/"
    queue: "q"
    prefetch: 100
    ttl: "30s"
    tls:
      enabled: false
  kafka:
    enabled: true
    brokers: ["127.0.0.1:9092"]
    topic: "t"
    group: "g"
    # acks omitted on purpose to test default="all"
`

func TestLoad_Valid(t *testing.T) {
	cfg, err := Load([]byte(validYAML))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.App.LogLevel != "info" {
		t.Fatalf("log_level defaulting failed")
	}
	if cfg.Connectors == (ConnectorsConfig{}) {
		t.Fatalf("connectors must be present")
	}
	if cfg.Connectors.Kafka.Acks != "all" {
		t.Fatalf("kafka.acks default=all not applied")
	}
	if cfg.Connectors.RabbitMQ.Prefetch != 100 {
		t.Fatalf("prefetch not parsed")
	}
}

func TestValidate_MissingKafkaTopic(t *testing.T) {
	bad := strings.ReplaceAll(validYAML, "topic: \"t\"", "")
	_, err := Load([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "connectors.kafka.topic is required") {
		t.Fatalf("expected error for missing topic, got %v", err)
	}
}

func TestValidate_MissingRabbitMQUrl(t *testing.T) {
	bad := strings.ReplaceAll(validYAML, "amqp://user:pass@localhost:5672/", "")
	_, err := Load([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "connectors.rabbitmq.url:") {
		t.Fatalf("expected error for missing utl, got %v", err)
	}
}

func TestValidate_KafkaBrokerHostPort(t *testing.T) {
	bad := strings.ReplaceAll(validYAML, "127.0.0.1:9092", "localhost")
	_, err := Load([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "connectors.kafka.brokers[localhost]") {
		t.Fatalf("expected host:port error, got %v", err)
	}
}

func TestValidate_RabbitTLSRequiresFiles(t *testing.T) {
	// turn on TLS without cert/key
	bad := strings.ReplaceAll(validYAML, "enabled: false", "enabled: true")
	_, err := Load([]byte(bad))
	if err == nil || !(strings.Contains(err.Error(), ".cert_file is required") && strings.Contains(err.Error(), ".key_file is required")) {
		t.Fatalf("expected cert/key required errors, got %v", err)
	}
}

func TestValidate_BadTTL(t *testing.T) {
	bad := strings.ReplaceAll(validYAML, "ttl: \"30s\"", "ttl: \"zzz\"")
	_, err := Load([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid duration, got %v", err)
	}
}
