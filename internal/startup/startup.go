package startup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/executil"
)

// Run executes configured startup hooks sequentially.
func Run(ctx context.Context, hooks []dsl.HookConfig, logger *slog.Logger) error {
	for idx, hook := range hooks {
		if strings.TrimSpace(hook.Command) == "" {
			continue
		}
		hookCtx := ctx
		var cancel context.CancelFunc
		if strings.TrimSpace(hook.Timeout) != "" {
			timeout, err := time.ParseDuration(hook.Timeout)
			if err != nil {
				return fmt.Errorf("startup hook %d: invalid timeout: %w", idx, err)
			}
			hookCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		if logger != nil {
			logger.Info("running startup hook", "index", idx)
		}

		output, _, err := executil.RunCommand(hookCtx, hook.Command, hook.Args, hook.Env, executil.TemplateData{})
		if err != nil {
			if logger != nil && strings.TrimSpace(output) != "" {
				logger.Error("startup hook failed", "index", idx, "output", strings.TrimSpace(output))
			}
			if cancel != nil {
				cancel()
			}
			return fmt.Errorf("startup hook %d failed: %w", idx, err)
		}
		if cancel != nil {
			cancel()
		}
		if logger != nil && strings.TrimSpace(output) != "" {
			logger.Info("startup hook output", "index", idx, "output", strings.TrimSpace(output))
		}
	}
	return nil
}
