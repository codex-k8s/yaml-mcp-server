package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/http/health"
	"github.com/codex-k8s/yaml-mcp-server/internal/timeutil"
)

// App controls the HTTP server lifecycle.
type App struct {
	baseCtx         context.Context
	server          *http.Server
	health          *health.Handler
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// New initializes the HTTP server with health endpoints.
func New(baseCtx context.Context, serverCfg dsl.ServerConfig, handler http.Handler, extra map[string]http.Handler, logger *slog.Logger, shutdownTimeout time.Duration) (*App, error) {
	if handler == nil {
		return nil, fmt.Errorf("handler is nil")
	}
	if baseCtx == nil {
		return nil, fmt.Errorf("base context is nil")
	}

	readTimeout := timeutil.ParseDurationOrDefault(serverCfg.HTTP.ReadTimeout, 15*time.Second)
	writeTimeout := timeutil.ParseDurationOrDefault(serverCfg.HTTP.WriteTimeout, 15*time.Second)
	idleTimeout := timeutil.ParseDurationOrDefault(serverCfg.HTTP.IdleTimeout, 60*time.Second)

	healthHandler := health.New()
	mux := http.NewServeMux()
	mux.Handle(serverCfg.HTTP.Path, handler)
	mux.HandleFunc("/healthz", healthHandler.Healthz)
	mux.HandleFunc("/readyz", healthHandler.Readyz)
	for path, route := range extra {
		if strings.TrimSpace(path) == "" || route == nil {
			continue
		}
		mux.Handle(path, route)
	}

	addr := net.JoinHostPort(strings.TrimSpace(serverCfg.HTTP.Host), strconv.Itoa(serverCfg.HTTP.Port))
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	if shutdownTimeout == 0 {
		shutdownTimeout = timeutil.ParseDurationOrDefault(serverCfg.ShutdownTimeout, 10*time.Second)
	}

	return &App{
		baseCtx:         baseCtx,
		server:          srv,
		health:          healthHandler,
		logger:          logger,
		shutdownTimeout: shutdownTimeout,
	}, nil
}

// Run starts the HTTP server and blocks until shutdown.
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.health.SetReady()
		if a.logger != nil {
			a.logger.Info("http server started", "addr", a.server.Addr)
		}
		errCh <- a.server.ListenAndServe()
	}()

	for {
		select {
		case <-ctx.Done():
			if a.logger != nil {
				a.logger.Info("shutdown requested")
			}
			return a.shutdown()
		case err := <-errCh:
			if err == nil || errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			if a.logger != nil {
				a.logger.Error("http server error", "error", err)
			}
			return err
		}
	}
}

func (a *App) shutdown() error {
	a.health.SetNotReady()
	ctx, cancel := context.WithTimeout(a.baseCtx, a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
