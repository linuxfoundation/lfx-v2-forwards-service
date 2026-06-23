// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	msg := nats.NewMsg(subject)
	msg.Header = make(nats.Header)
	msg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))

	reply, err := c.conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, newServiceUnavailable("NATS request failed", err)
	}
	return reply.Data, nil
}

// QueueSubscribe registers a core-NATS queue-group subscriber and returns an
// unsubscribe function the caller must invoke on shutdown.
// The handler receives the span context extracted from incoming message headers.
func (c *Client) QueueSubscribe(subject, queue string, handler func(ctx context.Context, msg *nats.Msg)) (func(), error) {
	sub, err := c.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		msgCtx := otel.GetTextMapPropagator().Extract(context.Background(), natsHeaderCarrier(msg.Header))
		msgCtx, span := tracer.Start(msgCtx, "nats.process",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "nats"),
				attribute.String("messaging.destination.name", subject),
				attribute.String("messaging.operation.type", "process"),
				attribute.Int("messaging.message.body.size", len(msg.Data)),
			),
		)
		defer span.End()
		handler(msgCtx, msg)
	})
	if err != nil {
		return nil, newServiceUnavailable("failed to subscribe to "+subject, err)
	}
	return func() { _ = sub.Unsubscribe() }, nil
}
