// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package authservice provides a NATS request/reply client for lfx-v2-auth-service.
// Types mirror the auth-service wire contract locally to avoid a cross-repo module dep.
package authservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	natsclient "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/nats"
)

// ErrNoAliasForDomain is returned when the caller has no identity for the requested domain.
var ErrNoAliasForDomain = errors.New("no alias found for user on requested domain")

// Client calls lfx-v2-auth-service via NATS request/reply.
type Client struct {
	nats    *natsclient.Client
	subject string
}

// New creates an authservice client.
// subject should be the value of AUTH_SERVICE_SUBJECT (default: lfx.auth-service.user_emails.read).
func New(nats *natsclient.Client, subject string) *Client {
	if subject == "" {
		subject = "lfx.auth-service.user_emails.read"
	}
	return &Client{nats: nats, subject: subject}
}

// getUserEmailsRequest mirrors auth-service's userEmailsRequest wire type.
type getUserEmailsRequest struct {
	User struct {
		AuthToken string `json:"auth_token"`
	} `json:"user"`
}

// getUserEmailsReply mirrors auth-service's UserDataResponse + data fields for user_emails.read.
type getUserEmailsReply struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    *struct {
		PrimaryEmail    string      `json:"primary_email"`
		AlternateEmails []emailItem `json:"alternate_emails"`
	} `json:"data,omitempty"`
}

type emailItem struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

// GetAliasForDomain calls auth-service to look up the caller's alias on the given domain.
// It returns the local part of the alias (e.g. "johndoe" for "johndoe@linux.com")
// or ErrNoAliasForDomain if the user has no such identity.
func (c *Client) GetAliasForDomain(ctx context.Context, authToken, domain string) (string, error) {
	reqBody := getUserEmailsRequest{}
	reqBody.User.AuthToken = authToken

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal auth-service request: %w", err)
	}

	raw, err := c.nats.Request(ctx, c.subject, data)
	if err != nil {
		return "", fmt.Errorf("auth-service request failed: %w", err)
	}

	var reply getUserEmailsReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return "", fmt.Errorf("unmarshal auth-service reply: %w", err)
	}

	if !reply.Success || reply.Error != "" {
		return "", fmt.Errorf("auth-service error: %s", reply.Error)
	}

	if reply.Data == nil {
		return "", ErrNoAliasForDomain
	}

	suffix := "@" + strings.ToLower(strings.TrimSpace(domain))
	for _, e := range reply.Data.AlternateEmails {
		lower := strings.ToLower(strings.TrimSpace(e.Email))
		if strings.HasSuffix(lower, suffix) {
			localPart := strings.TrimSuffix(lower, suffix)
			if localPart != "" {
				return localPart, nil
			}
		}
	}

	return "", ErrNoAliasForDomain
}
