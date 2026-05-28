// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"log/slog"

	authsvc "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/authservice"
	femail "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/forwardemail"
	jwtpkg "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/jwt"
	natsinfra "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/service"
)

// Package-level singletons populated by InitInfrastructure and torn down by Shutdown.
var (
	// NATSClient is the shared NATS connection used by all infrastructure adapters.
	NATSClient *natsinfra.Client
	// ForwardSvc is the wired forwards service.
	ForwardSvc *service.ForwardService
)

// InitInfrastructure initialises all infrastructure dependencies from cfg.
// Must be called once during startup before StartSubscriptions.
func InitInfrastructure(ctx context.Context, cfg AppConfig) error {
	nc, err := natsinfra.New(ctx, cfg.NATSURL, cfg.NATSCredentialsFile)
	if err != nil {
		return fmt.Errorf("init NATS: %w", err)
	}
	NATSClient = nc

	// Load JWKS from Auth0 for JWT verification.
	jwtCfg, err := jwtpkg.NewConfigFromJWKS(ctx, cfg.Auth0Domain, cfg.Auth0Audience)
	if err != nil {
		return fmt.Errorf("load JWKS from Auth0 (%s): %w", cfg.Auth0Domain, err)
	}

	authClient := authsvc.New(NATSClient, cfg.AuthServiceSubject)
	feClient := femail.New(cfg.ForwardEmailAPIToken, cfg.ForwardEmailBaseURL)

	ForwardSvc, err = service.New(service.Config{
		JWTConfig:          jwtCfg,
		AuthClient:         authClient,
		FEmailClient:       feClient,
		Domains:            cfg.ForwardsDomains,
		ExtraReserved:      cfg.ForwardsReservedNames,
		AuthServiceTimeout: cfg.AuthServiceRequestTimeout,
	})
	if err != nil {
		return fmt.Errorf("init forward service: %w", err)
	}

	slog.InfoContext(ctx, "infrastructure initialised",
		"domains", cfg.ForwardsDomains,
		"auth_service_subject", cfg.AuthServiceSubject,
	)
	return nil
}

// Shutdown gracefully closes all infrastructure connections.
func Shutdown() {
	if NATSClient != nil {
		NATSClient.Close()
	}
}
