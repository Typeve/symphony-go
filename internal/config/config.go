package config

import (
	"fmt"
	"os"

	"github.com/local/symphony/internal/domain"
	"gopkg.in/yaml.v3"
)

// Load reads a YAML config file, expands ${ENV_VAR} references, and returns
// a fully resolved domain.Config.
func Load(path string) (domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Config{}, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg domain.Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return domain.Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}
