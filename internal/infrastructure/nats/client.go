// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// Client wraps the NATS connection and provides infrastructure operations.
type Client struct {
	conn *nats.Conn
}

// New creates a NATS client connected to the given URL.
// If credentialsFile is non-empty, NKey credentials are used for authentication.
func New(ctx context.Context, url, credentialsFile string) (*Client, error) {
	opts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.ErrorContext(ctx, "NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.InfoContext(ctx, "NATS reconnected", "url", nc.ConnectedUrl())
		}),
	}
	if credentialsFile != "" {
		opts = append(opts, nats.UserCredentials(credentialsFile))
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, newServiceUnavailable("failed to connect to NATS", err)
	}

	slog.InfoContext(ctx, "NATS connected", "url", conn.ConnectedUrl())
	return &Client{conn: conn}, nil
}

// Close drains and closes the NATS connection.
func (c *Client) Close() {
	if c.conn != nil {
		_ = c.conn.Drain()
	}
}

// IsReady returns an error if the connection is not usable.
func (c *Client) IsReady() error {
	if c.conn == nil || !c.conn.IsConnected() || c.conn.IsDraining() {
		return newServiceUnavailable("NATS client is not ready")
	}
	return nil
}

// Request sends a synchronous NATS request and returns the raw response bytes.
func (c *Client) Request(ctx context.Context, subject string, data []byte) ([]byte, error) {
	msg, err := c.conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return nil, newServiceUnavailable("NATS request failed", err)
	}
	return msg.Data, nil
}

// QueueSubscribe registers a core-NATS queue-group subscriber and returns an
// unsubscribe function the caller must invoke on shutdown.
func (c *Client) QueueSubscribe(subject, queue string, handler nats.MsgHandler) (func(), error) {
	sub, err := c.conn.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, newServiceUnavailable("failed to subscribe to "+subject, err)
	}
	return func() { _ = sub.Unsubscribe() }, nil
}
