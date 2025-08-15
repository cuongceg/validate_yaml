package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"gopkg.in/yaml.v3"
)

func Load(path string, strict bool) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(strict)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	return &cfg, nil
}

// ApplyEnvOverride nạp ENV có prefix (vd APP_) và ghi đè vào *cfg.
// ENV không hợp lệ sẽ bị bỏ qua, không gây lỗi unknown key (an toàn).
func ApplyEnvOverride(cfg *Config, prefix string) error {
	k := koanf.New(".")
	mapper := func(s string) string {
		// APP_APP__LOG_LEVEL -> app.log_level (chấp nhận cả APP_APP_LOG_LEVEL)
		s = strings.TrimPrefix(s, prefix)
		s = strings.TrimPrefix(s, "_")
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "__", ".")
		s = strings.ReplaceAll(s, "_", ".")
		return s
	}
	if err := k.Load(env.Provider(prefix, ".", mapper), nil); err != nil {
		return err
	}
	// Unmarshal đè lên struct đã có (đã default trước đó)
	return k.Unmarshal("", cfg)
}

// nhỏ gọn: bytes.Reader tiện dụng
func bytesNewReader(b []byte) io.Reader { return bytes.NewReader(b) }
