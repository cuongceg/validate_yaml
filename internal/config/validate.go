package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	if c.SchemaVersion <= 0 {
		errs = append(errs, "schema_version must be >= 1")
	}

	if len(c.Routes) == 0 {
		errs = append(errs, "routes must not be empty")
	}
	for i, r := range c.Routes {
		path := fmt.Sprintf("routes[%d]", i)

		if r.From == "" {
			errs = append(errs, path+".from is required")
		} else if !oneOf(r.From, "rabbit", "kafka", "nats") {
			errs = append(errs, fmt.Sprintf("%s.from=%q must be one of rabbit|kafka|nats", path, r.From))
		} else if slices.Contains(r.To, r.From) {
			errs = append(errs, fmt.Sprintf("%s.from=%q must not be the same as any target in to", path, r.From))
		}

		if r.Direction == Forward || r.Direction == Both {
			if len(r.To) == 0 {
				errs = append(errs, path+".to must not be empty when direction=forward|both")
			} else {
				for _, t := range r.To {
					if !oneOf(t, "rabbit", "kafka", "nats") {
						errs = append(errs, fmt.Sprintf("%s.to contains invalid target %q (valid: rabbit|kafka|nats)", path, t))
					}
				}
			}
		}

		if r.QoS.MaxInflight < 1 {
			errs = append(errs, fmt.Sprintf("%s.qos.max_inflight must be >= 1", path))
		}
		if r.TTL.Timeout.Duration < 0 {
			errs = append(errs, fmt.Sprintf("%s.ttl.timeout must be >= 0", path))
		}
		if r.TTL.MaxHops < 0 {
			errs = append(errs, fmt.Sprintf("%s.ttl.max_hops must be >= 0", path))
		}

		for j, f := range r.Filters {
			fpath := fmt.Sprintf("%s.filters[%d]", path, j)
			switch f.Type {
			case "drop", "minimum":
				// ok
			default:
				errs = append(errs, fmt.Sprintf("%s.type=%q invalid (valid: drop|minimum)", fpath, f.Type))
			}
			// optional: topics regex/glob check ở bước sau
			if f.Type == "minimum" && f.Window.Duration <= 0 {
				errs = append(errs, fmt.Sprintf("%s.window must be > 0 for type=minimum", fpath))
			}
		}
	}

	switch c.Compression {
	case "none", "snappy", "zstd", "":
	default:
		errs = append(errs, fmt.Sprintf("compression=%q invalid (valid: none|snappy|zstd)", c.Compression))
	}

	// Adapter “tối thiểu” – không bắt buộc phải khai báo tất cả
	if c.Rabbit.URL == "" && c.Kafka.Brokers == nil && c.NATS.URL == "" {
		errs = append(errs, "at least one adapter (rabbit/kafka/nats) must be configured")
	}

	if c.Rabbit.TLS.InsecureSkipVerify || c.Kafka.TLS.InsecureSkipVerify || c.NATS.TLS.InsecureSkipVerify {
		fmt.Println("WARNING: TLS InsecureSkipVerify is enabled. This is insecure and should only be used for testing purposes.")
	}

	if c.Rabbit.TLS.Enabled && !c.Rabbit.TLS.InsecureSkipVerify {
		if c.Rabbit.TLS.CertFile == "" || c.Rabbit.TLS.KeyFile == "" || c.Rabbit.TLS.CAFile == "" {
			errs = append(errs, "rabbit.tls.cert_file, rabbit.tls.key_file and rabbit.tls.cafile must be set when TLS is enabled")
		}
	}

	if c.Kafka.TLS.Enabled && !c.Kafka.TLS.InsecureSkipVerify {
		if c.Kafka.TLS.CertFile == "" || c.Kafka.TLS.KeyFile == "" || c.Kafka.TLS.CAFile == "" {
			errs = append(errs, "kafka.tls.cert_file, kafka.tls.key_file and kafka.tls.cafile must be set when TLS is enabled")
		}
	}

	if c.NATS.TLS.Enabled && !c.NATS.TLS.InsecureSkipVerify {
		if c.NATS.TLS.CertFile == "" || c.NATS.TLS.KeyFile == "" || c.NATS.TLS.CAFile == "" {
			errs = append(errs, "nats.tls.cert_file, nats.tls.key_file and nats.tls.cafile must be set when TLS is enabled")
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func oneOf(s string, set ...string) bool {
	for _, v := range set {
		if s == v {
			return true
		}
	}
	return false
}
