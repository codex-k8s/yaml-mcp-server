package dsl

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// Load parses YAML bytes into Config and validates it.
func Load(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Load(data, &cfg, yaml.WithKnownFields()); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
