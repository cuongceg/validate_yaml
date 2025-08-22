package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*UserConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't read the config file %s: %w", path, err)
	}

	var usrConf UserConfig
	if err := yaml.Unmarshal([]byte(raw), &usrConf); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	validate := ValidateConfig(&usrConf)

	if validate != nil {
		return nil, fmt.Errorf("error configuration: %s", validate.Error())
	}
	return &usrConf, nil
}
