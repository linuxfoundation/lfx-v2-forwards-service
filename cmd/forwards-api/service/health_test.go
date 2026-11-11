// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	errDown := errors.New("down")

	tests := []struct {
		name   string
		method string
		path   string
		ready  func() error
		want   int
	}{
		{"livez ok", http.MethodGet, "/livez", func() error { return nil }, http.StatusOK},
		{"livez ignores readiness", http.MethodGet, "/livez", func() error { return errDown }, http.StatusOK},
		{"readyz ready", http.MethodGet, "/readyz", func() error { return nil }, http.StatusOK},
		{"readyz not ready", http.MethodGet, "/readyz", func() error { return errDown }, http.StatusServiceUnavailable},
		{"unknown path", http.MethodGet, "/other", func() error { return nil }, http.StatusNotFound},
		{"livez rejects POST", http.MethodPost, "/livez", func() error { return nil }, http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			HealthHandler(tt.ready).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Errorf("%s %s: got status %d, want %d", tt.method, tt.path, rec.Code, tt.want)
			}
		})
	}
}

func TestReady(t *testing.T) {
	t.Cleanup(func() {
		started.Store(false)
		NATSClient = nil
	})

	started.Store(false)
	NATSClient = nil
	if err := Ready(); !errors.Is(err, ErrNotReady) {
		t.Errorf("before MarkStarted: Ready() = %v, want ErrNotReady", err)
	}

	// Started but no NATS client wired must not panic and must report not ready.
	MarkStarted()
	if err := Ready(); !errors.Is(err, ErrNotReady) {
		t.Errorf("started without NATSClient: Ready() = %v, want ErrNotReady", err)
	}
}

func TestAppConfigFromEnv_Port(t *testing.T) {
	// Required vars so AppConfigFromEnv does not error.
	t.Setenv("FORWARDEMAIL_API_TOKEN", "token")
	t.Setenv("FORWARDS_DOMAINS", "example.org")
	t.Setenv("AUTH0_DOMAIN", "example.auth0.com")
	t.Setenv("AUTH0_AUDIENCE", "aud")

	t.Setenv("PORT", "")
	cfg, err := AppConfigFromEnv()
	if err != nil {
		t.Fatalf("AppConfigFromEnv: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("default Port = %q, want 8080", cfg.Port)
	}

	t.Setenv("PORT", "9090")
	cfg, err = AppConfigFromEnv()
	if err != nil {
		t.Fatalf("AppConfigFromEnv: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}
