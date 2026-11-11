// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"errors"
	"net/http"
	"sync/atomic"
)

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
