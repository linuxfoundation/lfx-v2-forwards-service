// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appsvc "github.com/linuxfoundation/lfx-v2-forwards-service/cmd/forwards-api/service"
	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/observability"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const gracefulShutdownSeconds = 25

func init() {
	observability.InitStructureLogConfig("")
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if os.Getenv("OTEL_SERVICE_VERSION") == "" {
		_ = os.Setenv("OTEL_SERVICE_VERSION", Version)
	}
	otelShutdown, err := observability.SetupOTelSDK(ctx)
	if err != nil {
		return err
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()
		if shutErr := otelShutdown(shutCtx); shutErr != nil {
			slog.ErrorContext(ctx, "error shutting down OpenTelemetry SDK", "error", shutErr)
		}
	}()

	slog.InfoContext(ctx, "starting forwards service",
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit,
	)

	cfg, err := appsvc.AppConfigFromEnv()
	if err != nil {
		return err
	}
	observability.InitStructureLogConfig(cfg.LogLevel)

	// Start the health server first so /livez answers while NATS connects and
	// JWKS loads. /readyz reports not-ready until startup has finished, then
	// tracks the NATS connection state.
	healthServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           appsvc.HealthHandler(appsvc.Ready),
		ReadHeaderTimeout: 5 * time.Second,
	}
	healthErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "health server listening", "port", cfg.Port)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			healthErr <- err
		}
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutErr := healthServer.Shutdown(shutCtx); shutErr != nil {
			slog.ErrorContext(ctx, "error shutting down health server", "error", shutErr)
		}
	}()

	if err := appsvc.InitInfrastructure(ctx, cfg); err != nil {
		return err
	}
	defer appsvc.Shutdown()

	stops, err := appsvc.StartSubscriptions(ctx)
	if err != nil {
		return err
	}
	defer func() {
		for _, stop := range stops {
			stop()
		}
	}()

	appsvc.MarkStarted()
	slog.InfoContext(ctx, "forwards service ready")

	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "received shutdown signal, stopping")
		return nil
	case err := <-healthErr:
		return fmt.Errorf("health server failed: %w", err)
	}
}
