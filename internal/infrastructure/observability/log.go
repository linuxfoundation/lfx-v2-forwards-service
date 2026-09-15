// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package observability

import (
	"context"
	"log"
	"log/slog"
	"os"

	slogotel "github.com/remychantenay/slog-otel"
)

type ctxKey string

const (
	slogFields      ctxKey = "slog_fields"
	logLevelDefault        = slog.LevelInfo
	debug                  = "debug"
	warn                   = "warn"
	info                   = "info"
	errorLvl               = "error"
)

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(slogFields).([]slog.Attr); ok {
		for _, v := range attrs {
			r.AddAttrs(v)
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}

// InitStructureLogConfig initializes the structured log configuration.
// logLevel should be "debug", "info", "warn", "error", or "" for the default (info).
// Call with "" during init() for early startup logging, then call again after
// AppConfigFromEnv() to apply the configured level.
func InitStructureLogConfig(logLevel string) {
	level := new(slog.LevelVar)
	level.Set(logLevelDefault)

	switch logLevel {
	case debug:
		level.Set(slog.LevelDebug)
	case info:
		level.Set(slog.LevelInfo)
	case warn:
		level.Set(slog.LevelWarn)
	case errorLvl:
		level.Set(slog.LevelError)
	default:
		level.Set(logLevelDefault)
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := contextHandler{slogotel.OtelHandler{
		Next: slog.NewJSONHandler(os.Stderr, opts),
	}}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetOutput(os.Stderr)
}
