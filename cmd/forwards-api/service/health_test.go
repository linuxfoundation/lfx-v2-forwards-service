// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestListenHealth(t *testing.T) {
	// A free port (0 = OS-assigned) binds.
	ln, err := ListenHealth("0")
	if err != nil {
		t.Fatalf("ListenHealth(0) = %v, want nil", err)
	}
	defer ln.Close() //nolint:errcheck

	// The port is now taken: binding it again must fail with a clear error.
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	if _, err := ListenHealth(port); err == nil {
		t.Errorf("ListenHealth(%s) on a busy port = nil, want error", port)
	}

	// An invalid port fails too.
	if _, err := ListenHealth("not-a-port"); err == nil {
		t.Error("ListenHealth(not-a-port) = nil, want error")
	}
}

func TestFlushWithRetry(t *testing.T) {
	errDown := errors.New("broker down")

	t.Run("succeeds immediately without retrying", func(t *testing.T) {
		calls := 0
		err := flushWithRetry(context.Background(), time.Millisecond, func() error {
			calls++
			return nil
		})
		if err != nil || calls != 1 {
			t.Errorf("got err=%v calls=%d, want nil and 1 call", err, calls)
		}
	})

	t.Run("retries until the flush succeeds", func(t *testing.T) {
		calls := 0
		err := flushWithRetry(context.Background(), time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errDown
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Errorf("got err=%v calls=%d, want nil after 3 calls", err, calls)
		}
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		err := flushWithRetry(ctx, time.Millisecond, func() error {
			calls++
			if calls == 2 {
				cancel()
			}
			return errDown
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled", err)
		}
		if calls != 2 {
			t.Errorf("flush called %d times, want 2 (stop after cancel)", calls)
		}
	})
}

func TestFlushSubscriptions_NoClientKeepsRetryingUntilCancelled(t *testing.T) {
	NATSClient = nil
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := FlushSubscriptions(ctx, time.Second, 5*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("FlushSubscriptions without NATSClient = %v, want context.DeadlineExceeded", err)
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
