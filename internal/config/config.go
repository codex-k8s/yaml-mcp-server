package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config stores environment-driven settings for the server.
type Config struct {
	// ConfigPath is the path to the YAML configuration file.
	ConfigPath string `env:"YAML_MCP_CONFIG" envDefault:"config.yaml"`
	// LogLevel sets the logger level.
	LogLevel string `env:"YAML_MCP_LOG_LEVEL" envDefault:"info"`
	// Lang selects message language for templates.
	Lang string `env:"YAML_MCP_LANG" envDefault:"en"`
	// ShutdownTimeout controls graceful shutdown duration.
	ShutdownTimeout time.Duration `env:"YAML_MCP_SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

// Load parses environment variables into Config.
func Load() (Config, error) {
	return env.ParseAs[Config]()
}
