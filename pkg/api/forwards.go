// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package api contains the public contract types and NATS subjects for
// lfx-v2-forwards-service. These are the only exported types intended for
// inter-service use; all other types remain internal.
package api

import "time"

// Subjects consumed by the forwards service.
const (
	// CheckAliasSubject is the NATS request/reply subject for checking alias availability
	// in forwardemail.net. No authentication required.
	CheckAliasSubject = "lfx.forwards-service.check_alias"

	// SetTargetSubject is the NATS request/reply subject for creating or updating the
	// forwarding routing for the caller's alias on the requested domain. JWT authentication required.
	SetTargetSubject = "lfx.forwards-service.set_target"

	// GetForwardSubject is the NATS request/reply subject for reading the current
	// forwarding routing for the caller's alias on the requested domain. JWT authentication required.
	GetForwardSubject = "lfx.forwards-service.get_forward"
)

// CheckAliasRequest is the payload for CheckAliasSubject.
type CheckAliasRequest struct {
	// Alias is the local part only (e.g. "johndoe", not "johndoe@linux.com").
	Alias string `json:"alias"`
	// Domain is the email domain to check against (e.g. "linux.com"). Required.
	Domain string `json:"domain"`
}

// CheckAliasReply is the reply payload for CheckAliasSubject.
// Error is set on validation failure.
type CheckAliasReply struct {
	Exists bool   `json:"exists"`
	Alias  string `json:"alias,omitempty"` // normalised (lowercased) alias
	Error  string `json:"error,omitempty"`
}

// SetTargetRequest is the payload for SetTargetSubject.
type SetTargetRequest struct {
	User struct {
		AuthToken string `json:"auth_token"`
	} `json:"user"`
	// Domain is the email domain to set the forwarding target for. Required.
	Domain      string `json:"domain"`
	TargetEmail string `json:"target_email"`
}

// SetTargetReply is the reply payload for SetTargetSubject.
type SetTargetReply struct {
	Alias       string     `json:"alias,omitempty"`
	TargetEmail string     `json:"target_email,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// GetForwardRequest is the payload for GetForwardSubject.
type GetForwardRequest struct {
	User struct {
		AuthToken string `json:"auth_token"`
	} `json:"user"`
	// Domain is the email domain to read the forwarding target for. Required.
	Domain string `json:"domain"`
}

// GetForwardReply is the reply payload for GetForwardSubject.
type GetForwardReply struct {
	Found       bool   `json:"found"`
	Alias       string `json:"alias,omitempty"`
	TargetEmail string `json:"target_email,omitempty"`
	Error       string `json:"error,omitempty"`
}
