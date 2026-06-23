// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-forwards-service/pkg/api"
)

const (
	forwardsQueueGroup = "forwards-service-workers"
	// msgHandlerTimeout caps the total wall time for one message, covering
	// JWT validation, auth-service round-trip, and forwardemail.net call.
	msgHandlerTimeout = 30 * time.Second
)

// StartSubscriptions binds all NATS subscribers and returns their stop functions.
// On a partial failure it unwinds any subscriptions already registered before
// returning the error, so a failed startup leaves no dangling subscriptions.
func StartSubscriptions(ctx context.Context) ([]func(), error) {
	stops := make([]func(), 0, 3)

	// stopAll unwinds the subscriptions registered so far. Used on partial failure.
	stopAll := func() {
		for _, stop := range stops {
			stop()
		}
	}

	subscribers := []struct {
		subject string
		bind    func(context.Context) (func(), error)
	}{
		{api.CheckAliasSubject, subscribeCheckAlias},
		{api.SetTargetSubject, subscribeSetTarget},
		{api.GetForwardSubject, subscribeGetForward},
	}

	for _, s := range subscribers {
		stop, err := s.bind(ctx)
		if err != nil {
			stopAll()
			return nil, err
		}
		stops = append(stops, stop)
		slog.InfoContext(ctx, "subscription started", "subject", s.subject)
	}

	return stops, nil
}

func subscribeCheckAlias(ctx context.Context) (func(), error) {
	stop, err := NATSClient.QueueSubscribe(api.CheckAliasSubject, forwardsQueueGroup, func(ctx context.Context, msg *nats.Msg) {
		msgCtx, cancel := context.WithTimeout(ctx, msgHandlerTimeout)
		defer cancel()

		var req api.CheckAliasRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.ErrorContext(msgCtx, "check_alias: failed to unmarshal payload", "error", err)
			replyError(msgCtx, msg, "malformed_request")
			return
		}

		result, errCode := ForwardSvc.HandleCheckAlias(msgCtx, req.Alias, req.Domain)
		var resp api.CheckAliasReply
		if errCode != "" {
			resp.Error = errCode
		} else {
			resp.Exists = result.Exists
			resp.Alias = result.Alias
		}

		replyJSON(msgCtx, msg, resp)
		slog.InfoContext(msgCtx, "check_alias reply sent",
			"alias", resp.Alias,
			"exists", resp.Exists,
			"error", resp.Error,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", api.CheckAliasSubject, err)
	}
	return stop, nil
}

func subscribeSetTarget(ctx context.Context) (func(), error) {
	stop, err := NATSClient.QueueSubscribe(api.SetTargetSubject, forwardsQueueGroup, func(ctx context.Context, msg *nats.Msg) {
		msgCtx, cancel := context.WithTimeout(ctx, msgHandlerTimeout)
		defer cancel()

		var req api.SetTargetRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.ErrorContext(msgCtx, "set_target: failed to unmarshal payload", "error", err)
			replyError(msgCtx, msg, "malformed_request")
			return
		}

		result, errCode := ForwardSvc.HandleSetTarget(msgCtx, req.User.AuthToken, req.Domain, req.TargetEmail)
		var resp api.SetTargetReply
		if errCode != "" {
			resp.Error = errCode
		} else {
			resp.Alias = result.Alias
			resp.TargetEmail = result.TargetEmail
			resp.UpdatedAt = &result.UpdatedAt
		}

		replyJSON(msgCtx, msg, resp)
		slog.InfoContext(msgCtx, "set_target reply sent",
			"alias", resp.Alias,
			"error", resp.Error,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", api.SetTargetSubject, err)
	}
	return stop, nil
}

func subscribeGetForward(ctx context.Context) (func(), error) {
	stop, err := NATSClient.QueueSubscribe(api.GetForwardSubject, forwardsQueueGroup, func(ctx context.Context, msg *nats.Msg) {
		msgCtx, cancel := context.WithTimeout(ctx, msgHandlerTimeout)
		defer cancel()

		var req api.GetForwardRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.ErrorContext(msgCtx, "get_forward: failed to unmarshal payload", "error", err)
			replyError(msgCtx, msg, "malformed_request")
			return
		}

		result, errCode := ForwardSvc.HandleGetForward(msgCtx, req.User.AuthToken, req.Domain)
		var resp api.GetForwardReply
		if errCode != "" {
			resp.Error = errCode
		} else {
			resp.Found = result.Found
			resp.Alias = result.Alias
			resp.TargetEmail = result.TargetEmail
		}

		replyJSON(msgCtx, msg, resp)
		slog.InfoContext(msgCtx, "get_forward reply sent",
			"found", resp.Found,
			"alias", resp.Alias,
			"error", resp.Error,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", api.GetForwardSubject, err)
	}
	return stop, nil
}

// replyJSON marshals v and sends it as the NATS reply. Errors are logged.
func replyJSON(ctx context.Context, msg *nats.Msg, v interface{}) {
	if msg.Reply == "" {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal reply", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		slog.ErrorContext(ctx, "failed to send reply", "error", err)
	}
}

// replyError sends a minimal error-only JSON payload as the NATS reply.
func replyError(ctx context.Context, msg *nats.Msg, errCode string) {
	if msg.Reply == "" {
		return
	}
	data, _ := json.Marshal(map[string]string{"error": errCode})
	if err := msg.Respond(data); err != nil {
		slog.ErrorContext(ctx, "failed to send error reply", "error", err)
	}
}
