// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
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
	ctx := context.Background()

	otelConfig := observability.OTelConfigFromEnv(ctx)
	if otelConfig.ServiceVersion == "" {
		otelConfig.ServiceVersion = Version
	}
	otelShutdown, err := observability.SetupOTelSDKWithConfig(ctx, otelConfig)
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

	slog.InfoContext(ctx, "forwards service ready")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.InfoContext(ctx, "received shutdown signal, stopping", "signal", sig.String())
	return nil
}
