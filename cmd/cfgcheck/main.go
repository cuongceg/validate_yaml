package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/cuongceg/validate_yaml/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		cfgPath    string
		strict     bool
		printDef   bool
		format     string
		envPrefix  string
		showSample bool
	)
	flag.StringVar(&cfgPath, "config", "configs/example.yaml", "Đường dẫn file YAML cấu hình")
	flag.BoolVar(&strict, "strict", true, "Bật lỗi nếu YAML có key không khai báo (KnownFields)")
	flag.BoolVar(&printDef, "print-default", false, "In cấu hình sau khi ApplyDefaults() (đã merge ENV nếu có)")
	flag.StringVar(&format, "format", "text", "Định dạng output: text|json")
	flag.StringVar(&envPrefix, "env-prefix", "", "Tiền tố ENV để override (ví dụ: APP_)")
	flag.BoolVar(&showSample, "example", false, "In YAML mẫu rồi thoát")
	flag.Parse()

	if showSample {
		fmt.Println(sampleYAML())
		return
	}

	cfg, err := config.Load(cfgPath, strict)
	if err != nil {
		exitErr(format, fmt.Errorf("load: %w", err))
	}

	// Apply defaults trước
	cfg.ApplyDefaults()

	// ENV override (tuỳ chọn) – chỉ ghi đè field hợp lệ
	if envPrefix != "" {
		if err := config.ApplyEnvOverride(cfg, envPrefix); err != nil {
			exitErr(format, fmt.Errorf("env override: %w", err))
		}
	}

	// Validate cuối
	if err := cfg.Validate(); err != nil {
		exitErr(format, fmt.Errorf("validate: %w", err))
	}

	if printDef {
		out, _ := yaml.Marshal(cfg)
		switch format {
		case "json":
			// In JSON của YAML đã default
			var m map[string]any
			_ = yaml.Unmarshal(out, &m)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(m)
		default:
			fmt.Print(string(out))
		}
		return
	}

	switch format {
	case "json":
		resp := map[string]any{
			"status": "ok",
			"msg":    "config valid",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
	default:
		fmt.Println("OK: config valid")
	}
}

func exitErr(format string, err error) {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
	default:
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	}
	os.Exit(1)
}

func sampleYAML() string {
	return `schema_version: 1
app:
  log_level: info
  metrics_addr: ":9090"

routes:
  - from: rabbit
    to: [kafka, nats]
    direction: both
    filters:
      - type: drop
        topics: ["telemetry.*"]
      - type: minimum
        topics: ["metric.*"]
        window: "5s"
    qos:
      acks: at_least_once
      max_inflight: 1000
    ttl:
      timeout: "30s"
      max_hops: 3

rabbit:
  url: "amqps://user:pass@host/vhost"
  tls:
    enabled: true
    ca_file: "ca.pem"
    cert_file: "client.pem"
    key_file: "client.key"
    insecure_skip_verify: false

kafka:
  brokers: ["k1:9093","k2:9093"]
  tls:
    enabled: false

nats:
  url: "nats://n1:4222"
  tls:
    enabled: false

persistence:
  type: badger
  path: "./data"

protobuf:
  files: ["./proto/msg.proto"]
  msg_type: "telemetry.Message"

compression: none
`
}
