package config
import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

func Validate(c *Config) error {
	var errs []error

	// app
	switch c.App.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("app.log_level must be one of: debug|info|warn|error"))
	}

	// connectors presence
	if c.Connectors.RabbitMQ == nil && c.Connectors.Kafka == nil && c.Connectors.NATS == nil {
		errs = append(errs, errors.New("at least one connectors.[rabbitmq|kafka|nats] must be present"))
	}

	if r := c.Connectors.RabbitMQ; r != nil {
		if r.Enabled {
			if err := mustURLScheme(r.URL, "amqp", "amqps"); err != nil {
				errs = append(errs, fmt.Errorf("connectors.rabbitmq.url: %w", err))
			}
			if r.Queue == "" { errs = append(errs, errors.New("connectors.rabbitmq.queue is required")) }
			if r.Prefetch <= 0 { errs = append(errs, errors.New("connectors.rabbitmq.prefetch must be > 0")) }
			if _, err := ParseDuration(r.TTL); err != nil {
				errs = append(errs, fmt.Errorf("connectors.rabbitmq.ttl invalid duration: %v", err))
			}
			if err := validateTLS("connectors.rabbitmq.tls", r.TLS); err != nil { errs = append(errs, err) }
		}
	}

	if k := c.Connectors.Kafka; k != nil {
		if k.Enabled {
			if len(k.Brokers) == 0 { errs = append(errs, errors.New("connectors.kafka.brokers must have at least one host:port")) }
			for _, b := range k.Brokers {
				if err := hostPort(b); err != nil { errs = append(errs, fmt.Errorf("connectors.kafka.brokers[%s]: %v", b, err)) }
			}
			if k.Topic == "" { errs = append(errs, errors.New("connectors.kafka.topic is required")) }
			if k.Group == "" { errs = append(errs, errors.New("connectors.kafka.group is required")) }
			switch k.Acks { case "none", "one", "all": default:
				errs = append(errs, errors.New("connectors.kafka.acks must be one of: none|one|all")) }
			if k.LingerMS < 0 { errs = append(errs, errors.New("connectors.kafka.linger_ms must be >= 0")) }
			if err := validateTLS("connectors.kafka.tls", k.TLS); err != nil { errs = append(errs, err) }
		}
	}

	if n := c.Connectors.NATS; n != nil {
		if n.Enabled {
			if err := mustURLScheme(n.URL, "nats", "tls"); err != nil {
				errs = append(errs, fmt.Errorf("connectors.nats.url: %w", err))
			}
			if n.Stream == "" { errs = append(errs, errors.New("connectors.nats.stream is required")) }
			if n.Subject == "" { errs = append(errs, errors.New("connectors.nats.subject is required")) }
			if n.PullBatch <= 0 { errs = append(errs, errors.New("connectors.nats.pull_batch must be > 0")) }
			if err := validateTLS("connectors.nats.tls", n.TLS); err != nil { errs = append(errs, err) }
		}
	}

	// filters (syntactic checks only for project 1)
	for i, f := range c.Filters {
		if f.Name == "" { errs = append(errs, fmt.Errorf("filters[%d].name is required", i)) }
		switch f.Type { case "drop", "min": default:
			errs = append(errs, fmt.Errorf("filters[%d].type must be drop|min", i)) }
		if f.Match.Field == "" { errs = append(errs, fmt.Errorf("filters[%d].match.field is required", i)) }
		if f.Match.Op == "" { errs = append(errs, fmt.Errorf("filters[%d].match.op is required", i)) }
	}

	if len(errs) > 0 {
		return joinErrs(errs)
	}
	return nil
}

func validateTLS(prefix string, t TLSConfig) error {
	if !t.Enabled { return nil }
	var errs []error
	if t.CertFile == "" { errs = append(errs, fmt.Errorf("%s.cert_file is required when enabled=true", prefix)) }
	if t.KeyFile == "" { errs = append(errs, fmt.Errorf("%s.key_file is required when enabled=true", prefix)) }
	if len(errs) > 0 { return joinErrs(errs) }
	return nil
}

func mustURLScheme(raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil { return fmt.Errorf("invalid url: %v", err) }
	ok := false
	for _, s := range schemes { if u.Scheme == s { ok = true; break } }
	if !ok { return fmt.Errorf("url scheme must be one of %v", schemes) }
	return nil
}

func hostPort(s string) error {
	h, p, err := net.SplitHostPort(s)
	if err != nil { return err }
	if h == "" { return errors.New("empty host") }
	if p == "" { return errors.New("empty port") }
	return nil
}

// joinErrs renders multi-error nicely for CLI output.
func joinErrs(errs []error) error {
	msg := "config validation failed:\n"
	for _, e := range errs { msg += "  - " + e.Error() + "\n" }
	return errors.New(msg)
}