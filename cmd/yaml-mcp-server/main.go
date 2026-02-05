package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codex-k8s/yaml-mcp-server/configs"
	"github.com/codex-k8s/yaml-mcp-server/internal/app"
	approverhttp "github.com/codex-k8s/yaml-mcp-server/internal/approver/http"
	"github.com/codex-k8s/yaml-mcp-server/internal/audit"
	"github.com/codex-k8s/yaml-mcp-server/internal/config"
	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/idempotency"
	"github.com/codex-k8s/yaml-mcp-server/internal/log"
	"github.com/codex-k8s/yaml-mcp-server/internal/render"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime"
	runtimeexecutor "github.com/codex-k8s/yaml-mcp-server/internal/runtime/executor"
	"github.com/codex-k8s/yaml-mcp-server/internal/startup"
	"github.com/codex-k8s/yaml-mcp-server/internal/templates"
)

func main() {
	embeddedConfig := flag.String("embedded-config", "", "Use embedded config from configs/ (filename)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := log.New(cfg.LogLevel)

	var rendered []byte
	if embeddedConfig != nil && *embeddedConfig != "" {
		raw, err := configs.Load(*embeddedConfig)
		if err != nil {
			logger.Error("load embedded config failed", "error", err)
			os.Exit(1)
		}
		rendered, err = render.RenderBytes(*embeddedConfig, raw)
	} else {
		rendered, err = render.RenderFile(cfg.ConfigPath)
	}
	if err != nil {
		logger.Error("render config failed", "error", err)
		os.Exit(1)
	}

	dslCfg, err := dsl.Load(rendered)
	if err != nil {
		logger.Error("parse config failed", "error", err)
		os.Exit(1)
	}

	templateBundle, err := templates.Load(cfg.Lang)
	if err != nil {
		logger.Error("load templates failed", "error", err)
		os.Exit(1)
	}

	var cache *idempotency.Cache
	if dslCfg.Server.Idempotency.Enabled {
		ttl, err := time.ParseDuration(dslCfg.Server.Idempotency.TTL)
		if err != nil {
			logger.Error("invalid idempotency ttl", "error", err)
			os.Exit(1)
		}
		cache = idempotency.NewCache(ttl, dslCfg.Server.Idempotency.MaxEntries)
	}

	builder := runtime.Builder{
		Logger:             logger,
		Audit:              audit.New(logger),
		Templates:          templateBundle,
		Cache:              cache,
		CacheKeyStrategy:   dslCfg.Server.Idempotency.KeyStrategy,
		Lang:               cfg.Lang,
		ApprovalWebhookURL: dslCfg.Server.ApprovalWebhookURL,
		ExecutorWebhookURL: dslCfg.Server.ExecutorWebhookURL,
	}
	if hasAsyncHTTPApprover(dslCfg) {
		builder.HTTPApprovals = approverhttp.NewPendingStore()
	}
	if builder.HTTPApprovals == nil && strings.TrimSpace(dslCfg.Server.ApprovalWebhookURL) != "" {
		builder.HTTPApprovals = approverhttp.NewPendingStore()
	}
	if hasAsyncHTTPExecutor(dslCfg) {
		builder.HTTPExecutions = runtimeexecutor.NewPendingStore()
	}
	if builder.HTTPExecutions == nil && strings.TrimSpace(dslCfg.Server.ExecutorWebhookURL) != "" {
		builder.HTTPExecutions = runtimeexecutor.NewPendingStore()
	}
	server, err := builder.Build(dslCfg)
	if err != nil {
		logger.Error("build server failed", "error", err)
		os.Exit(1)
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		sig := <-sigCh
		logger.Warn("shutdown requested", "signal", sig.String())
		cancel()
	}()

	if err := startup.Run(baseCtx, dslCfg.Server.StartupHooks, logger); err != nil {
		logger.Error("startup hooks failed", "error", err)
		os.Exit(1)
	}

	switch dslCfg.Server.Transport {
	case "stdio":
		if err := runStdio(baseCtx, server); err != nil {
			logger.Error("runtime error", "error", err)
			os.Exit(1)
		}
		return
	default:
		if err := runHTTP(baseCtx, cfg, dslCfg, server, builder.HTTPApprovals, builder.HTTPExecutions, logger); err != nil {
			logger.Error("runtime error", "error", err)
			os.Exit(1)
		}
	}
}

func runStdio(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}

func runHTTP(
	ctx context.Context,
	envCfg config.Config,
	dslCfg *dsl.Config,
	server *mcp.Server,
	approvals *approverhttp.PendingStore,
	executions *runtimeexecutor.PendingStore,
	logger *slog.Logger,
) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: dslCfg.Server.HTTP.Stateless,
	})

	extra := map[string]http.Handler{}
	addRoute := func(path string, route http.Handler) error {
		if strings.TrimSpace(path) == "" || route == nil {
			return nil
		}
		if _, exists := extra[path]; exists {
			return fmt.Errorf("duplicate extra http route: %s", path)
		}
		extra[path] = route
		return nil
	}
	if strings.TrimSpace(dslCfg.Server.ApprovalWebhookURL) != "" {
		path := webhookPath(dslCfg.Server.ApprovalWebhookURL)
		if path != "" {
			if err := addRoute(path, &approverhttp.WebhookHandler{Store: approvals, Logger: logger}); err != nil {
				return err
			}
		}
	}
	for _, raw := range executorWebhookURLs(dslCfg) {
		path := webhookPath(raw)
		if path == "" {
			continue
		}
		if err := addRoute(path, &runtimeexecutor.WebhookHandler{Store: executions, Logger: logger}); err != nil {
			return err
		}
	}

	application, err := app.New(ctx, dslCfg.Server, handler, extra, logger, envCfg.ShutdownTimeout)
	if err != nil {
		return err
	}

	return application.Run(ctx)
}

func hasAsyncHTTPApprover(cfg *dsl.Config) bool {
	if cfg == nil {
		return false
	}
	for _, tool := range cfg.Tools {
		for _, approver := range tool.Approvers {
			if strings.EqualFold(approver.Type, "http") && approver.Async {
				return true
			}
		}
	}
	return false
}

func hasAsyncHTTPExecutor(cfg *dsl.Config) bool {
	if cfg == nil {
		return false
	}
	for _, tool := range cfg.Tools {
		if strings.EqualFold(tool.Executor.Type, "http") && tool.Executor.Async {
			return true
		}
	}
	return false
}

func executorWebhookURLs(cfg *dsl.Config) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 1)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}

	add(cfg.Server.ExecutorWebhookURL)
	for _, tool := range cfg.Tools {
		if !strings.EqualFold(tool.Executor.Type, "http") {
			continue
		}
		if !tool.Executor.Async {
			continue
		}
		if strings.TrimSpace(tool.Executor.WebhookURL) != "" {
			add(tool.Executor.WebhookURL)
			continue
		}
		add(cfg.Server.ExecutorWebhookURL)
	}
	return out
}

func webhookPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(parsed.Path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
