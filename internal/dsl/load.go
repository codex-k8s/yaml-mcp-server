package dsl

import (
	"fmt"

	"github.com/yaml/go-yaml"
)

// Load parses YAML bytes into Config and validates it.
func Load(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := normalizeConfig(&cfg); err != nil {
		return nil, fmt.Errorf("normalize config: %w", err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
