// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service provides configuration and dependency-injection wiring for
// the forwards-api binary. All environment variable reads live in this package;
// the rest of the codebase receives typed values.
package service

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// AppConfig holds all runtime configuration read from environment variables.
type AppConfig struct {
	// NATS
	NATSURL             string
	NATSCredentialsFile string

	// forwardemail.net
	ForwardEmailAPIToken string
	ForwardEmailBaseURL  string

	// Domain configuration — index 0 is the default domain for requests that omit domain.
	ForwardsDomains       []string
	ForwardsReservedNames []string

	// Auth0 JWT verification
	Auth0Domain   string
	Auth0Audience string

	// Auth-service NATS subject
	AuthServiceSubject        string
	AuthServiceRequestTimeout time.Duration

	// Logging
	LogLevel string
}

// AppConfigFromEnv reads AppConfig from environment variables, applying defaults
// where reasonable. Returns an error if any required variable is missing.
func AppConfigFromEnv() (AppConfig, error) {
	cfg := AppConfig{
		NATSURL:             envOr("NATS_URL", "nats://lfx-platform-nats.lfx.svc.cluster.local:4222"),
		NATSCredentialsFile: os.Getenv("NATS_CREDENTIALS_FILE"),

		ForwardEmailAPIToken: os.Getenv("FORWARDEMAIL_API_TOKEN"),
		ForwardEmailBaseURL:  os.Getenv("FORWARDEMAIL_BASE_URL"),

		ForwardsDomains:       forwardsDomains(),
		ForwardsReservedNames: parseCSV(os.Getenv("FORWARDS_RESERVED_NAMES")),

		Auth0Domain:   os.Getenv("AUTH0_DOMAIN"),
		Auth0Audience: os.Getenv("AUTH0_AUDIENCE"),

		AuthServiceSubject:        envOr("AUTH_SERVICE_SUBJECT", "lfx.auth-service.user_emails.read"),
		AuthServiceRequestTimeout: durationOr("AUTH_SERVICE_REQUEST_TIMEOUT", 5*time.Second),

		LogLevel: os.Getenv("LOG_LEVEL"),
	}

	var missing []string
	if cfg.ForwardEmailAPIToken == "" {
		missing = append(missing, "FORWARDEMAIL_API_TOKEN")
	}
	if cfg.Auth0Domain == "" {
		missing = append(missing, "AUTH0_DOMAIN")
	}
	if cfg.Auth0Audience == "" {
		missing = append(missing, "AUTH0_AUDIENCE")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// forwardsDomains reads FORWARDS_DOMAINS (CSV) with a fallback to the deprecated
// singular FORWARDS_DOMAIN. Defaults to ["linux.com"] when neither is set.
func forwardsDomains() []string {
	if v := os.Getenv("FORWARDS_DOMAINS"); strings.TrimSpace(v) != "" {
		return parseCSV(v)
	}
	// deprecated singular fallback
	if v := strings.TrimSpace(os.Getenv("FORWARDS_DOMAIN")); v != "" {
		return []string{v}
	}
	return []string{"linux.com"}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func durationOr(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Warn("invalid duration env var, using default", "key", key, "value", v, "default", fallback)
			return fallback
		}
		return d
	}
	return fallback
}
