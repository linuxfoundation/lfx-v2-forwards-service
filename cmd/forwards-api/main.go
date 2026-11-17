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

const (
	// subscriptionFlushTimeout bounds each NATS round trip that confirms the
	// broker has registered all subscriptions before the service reports ready.
	subscriptionFlushTimeout = 5 * time.Second
	// subscriptionFlushRetryInterval is the pause between failed flush attempts
	// while NATS is unavailable at startup.
	subscriptionFlushRetryInterval = time.Second
)

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
	// tracks the NATS connection state. Bind the port up front so a busy or
	// invalid port fails startup immediately.
	healthListener, err := appsvc.ListenHealth(cfg.Port)
	if err != nil {
		return err
	}
	healthServer := &http.Server{
		Handler:           appsvc.HealthHandler(appsvc.Ready),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	// startupCtx is cancelled if the health server fails, so the startup flush
	// retry below stops instead of waiting on a service that can't report ready.
	startupCtx, cancelStartup := context.WithCancel(ctx)
	defer cancelStartup()
	healthErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "health server listening", "port", cfg.Port)
		if err := healthServer.Serve(healthListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			healthErr <- err
			cancelStartup()
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

	// QueueSubscribe is asynchronous: confirm the broker has registered every
	// subscription before /readyz can report ready. The NATS client reconnects
	// on its own, so a broker that is down at startup is retried (with /readyz
	// still 503) rather than crash-looping the pod.
	if err := appsvc.FlushSubscriptions(startupCtx, subscriptionFlushTimeout, subscriptionFlushRetryInterval); err != nil {
		select {
		case herr := <-healthErr:
			return fmt.Errorf("health server failed: %w", herr)
		default:
		}
		if ctx.Err() != nil {
			slog.InfoContext(ctx, "received shutdown signal during startup, stopping")
			return nil
		}
		return fmt.Errorf("flush NATS subscriptions: %w", err)
	}

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
