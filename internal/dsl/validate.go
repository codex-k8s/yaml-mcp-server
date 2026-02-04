package dsl

import (
	"fmt"
	"strings"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/constants"
)

// Validate applies defaults and verifies required fields.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.Server.Name == "" {
		return fmt.Errorf("server.name is required")
	}
	if cfg.Server.Version == "" {
		return fmt.Errorf("server.version is required")
	}
	if cfg.Server.Transport == "" {
		cfg.Server.Transport = "http"
	}
	if cfg.Server.HTTP.Listen == "" {
		cfg.Server.HTTP.Listen = ":8080"
	}
	if cfg.Server.HTTP.Path == "" {
		cfg.Server.HTTP.Path = "/mcp"
	}
	if cfg.Server.Idempotency.Enabled {
		if cfg.Server.Idempotency.TTL == "" {
			cfg.Server.Idempotency.TTL = "1h"
		}
		if cfg.Server.Idempotency.MaxEntries == 0 {
			cfg.Server.Idempotency.MaxEntries = 1000
		}
		if cfg.Server.Idempotency.MaxEntries < 0 {
			return fmt.Errorf("server.idempotency_cache.max_entries must be >= 0")
		}
		if _, err := time.ParseDuration(cfg.Server.Idempotency.TTL); err != nil {
			return fmt.Errorf("server.idempotency_cache.ttl is invalid: %w", err)
		}
		if cfg.Server.Idempotency.KeyStrategy == "" {
			cfg.Server.Idempotency.KeyStrategy = constants.CacheKeyStrategyAuto
		}
		switch strings.ToLower(strings.TrimSpace(cfg.Server.Idempotency.KeyStrategy)) {
		case constants.CacheKeyStrategyAuto, constants.CacheKeyStrategyCorrelationID, constants.CacheKeyStrategyArgumentsHash:
		default:
			return fmt.Errorf("server.idempotency_cache.key_strategy must be auto, correlation_id, or arguments_hash")
		}
	}

	for i, hook := range cfg.Server.StartupHooks {
		if strings.TrimSpace(hook.Command) == "" {
			return fmt.Errorf("server.startup_hooks[%d].command is required", i)
		}
	}

	toolNames := map[string]struct{}{}
	for i, tool := range cfg.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tools[%d].name is required", i)
		}
		if _, exists := toolNames[tool.Name]; exists {
			return fmt.Errorf("duplicate tool name: %s", tool.Name)
		}
		toolNames[tool.Name] = struct{}{}
		if strings.TrimSpace(tool.Executor.Type) == "" {
			return fmt.Errorf("tools[%d].executor.type is required", i)
		}
		for j, approver := range tool.Approvers {
			if strings.TrimSpace(approver.Type) == "" {
				return fmt.Errorf("tools[%d].approvers[%d].type is required", i, j)
			}
		}
	}

	resourceURIs := map[string]struct{}{}
	for i, res := range cfg.Resources {
		if res.URI == "" {
			return fmt.Errorf("resources[%d].uri is required", i)
		}
		if _, exists := resourceURIs[res.URI]; exists {
			return fmt.Errorf("duplicate resource uri: %s", res.URI)
		}
		resourceURIs[res.URI] = struct{}{}
	}

	return nil
}
