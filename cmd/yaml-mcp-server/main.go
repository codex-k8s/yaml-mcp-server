package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codex-k8s/yaml-mcp-server/internal/app"
	"github.com/codex-k8s/yaml-mcp-server/internal/audit"
	"github.com/codex-k8s/yaml-mcp-server/internal/config"
	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/log"
	"github.com/codex-k8s/yaml-mcp-server/internal/render"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime"
	"github.com/codex-k8s/yaml-mcp-server/internal/startup"
	"github.com/codex-k8s/yaml-mcp-server/internal/templates"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := log.New(cfg.LogLevel)

	rendered, err := render.RenderFile(cfg.ConfigPath)
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

	builder := runtime.Builder{
		Logger:    logger,
		Audit:     audit.New(logger),
		Templates: templateBundle,
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
		if err := runHTTP(baseCtx, cfg, dslCfg, server, logger); err != nil {
			logger.Error("runtime error", "error", err)
			os.Exit(1)
		}
	}
}

func runStdio(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}

func runHTTP(ctx context.Context, envCfg config.Config, dslCfg *dsl.Config, server *mcp.Server, logger *slog.Logger) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: dslCfg.Server.HTTP.Stateless,
	})

	application, err := app.New(ctx, dslCfg.Server, handler, logger, envCfg.ShutdownTimeout)
	if err != nil {
		return err
	}

	return application.Run(ctx)
}
