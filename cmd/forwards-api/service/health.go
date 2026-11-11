// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// ListenHealth binds the health server's TCP port. Binding up front, before
// the slow startup work (NATS, JWKS), surfaces a busy or invalid port as an
// immediate error instead of a late failure.
func ListenHealth(port string) (net.Listener, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("listen on health port %q: %w", port, err)
	}
	return ln, nil
}

// FlushSubscriptions blocks until the broker has processed all subscriptions
// registered by StartSubscriptions. Subscribe calls are asynchronous, so call
// this after StartSubscriptions and before MarkStarted.
//
// The NATS client reconnects indefinitely, so a broker that is unavailable at
// startup is not fatal: each failed flush (bounded by timeout) is retried every
// retryInterval until it succeeds. It returns ctx.Err() if ctx is done first.
// Until it returns nil the caller has not called MarkStarted, so /readyz stays
// 503.
func FlushSubscriptions(ctx context.Context, timeout, retryInterval time.Duration) error {
	return flushWithRetry(ctx, retryInterval, func() error {
		if NATSClient == nil {
			return ErrNotReady
		}
		return NATSClient.Flush(timeout)
	})
}

// flushWithRetry calls flush until it succeeds, waiting retryInterval between
// attempts, and returns ctx.Err() if ctx is done first.
func flushWithRetry(ctx context.Context, retryInterval time.Duration, flush func() error) error {
	for {
		err := flush()
		if err == nil {
			return nil
		}
		slog.WarnContext(ctx, "NATS flush failed, retrying",
			"error", err,
			"retry_in", retryInterval,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}

// ErrNotReady is returned by Ready until startup has finished.
var ErrNotReady = errors.New("forwards service not ready")

// started is set once infrastructure is initialised and all subscriptions are
// registered. It also publishes the NATSClient singleton to the health
// goroutine: NATSClient is written before started.Store(true) and only read by
// Ready after started.Load() returns true.
var started atomic.Bool

// MarkStarted records that startup has finished (NATS connected, JWKS loaded
// and subscriptions registered). Call it once, after StartSubscriptions.
func MarkStarted() {
	started.Store(true)
}

// Ready reports whether the service can currently serve requests. It returns
// ErrNotReady until MarkStarted is called, then tracks the NATS connection
// (not connected or draining => error).
func Ready() error {
	if !started.Load() || NATSClient == nil {
		return ErrNotReady
	}
	return NATSClient.IsReady()
}

// HealthHandler returns an http.Handler serving the Kubernetes probe endpoints:
//
//   - GET /livez: always 200 while the process is serving HTTP. It does not
//     inspect NATS state, so a broker outage never restarts the pod.
//   - GET /readyz: 200 when ready returns nil, otherwise 503.
//
// The forwards service is a pure NATS subscriber, so these are the only HTTP
// routes it exposes.
func HealthHandler(ready func() error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if err := ready(); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
