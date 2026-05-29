// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	femail "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/forwardemail"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/authservice"
)

// ForwardService implements the business logic for the forwards service.
type ForwardService struct {
	jwtCfg             port.TokenVerifier
	authClient         port.AuthServiceClient
	feClient           port.ForwardEmailProvider
	domains            []string
	extraReserved      []string
	authServiceTimeout time.Duration
}

// Config holds the dependencies and settings for ForwardService.
type Config struct {
	JWTConfig          port.TokenVerifier
	AuthClient         port.AuthServiceClient
	FEmailClient       port.ForwardEmailProvider
	Domains            []string
	ExtraReserved      []string
	AuthServiceTimeout time.Duration
}

// New creates a ForwardService. Returns an error if Domains is empty.
func New(cfg Config) (*ForwardService, error) {
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("ForwardService requires at least one configured domain")
	}
	if cfg.JWTConfig == nil {
		return nil, fmt.Errorf("ForwardService requires a non-nil JWTConfig")
	}
	timeout := cfg.AuthServiceTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &ForwardService{
		jwtCfg:             cfg.JWTConfig,
		authClient:         cfg.AuthClient,
		feClient:           cfg.FEmailClient,
		domains:            cfg.Domains,
		extraReserved:      cfg.ExtraReserved,
		authServiceTimeout: timeout,
	}, nil
}

// resolveDomain returns the domain to use for a request. The requested value is
// required and must (case-insensitively) match one of the configured domains.
// Returns "domain_required" when empty, "domain_not_allowed" when not in the allow-list.
func (s *ForwardService) resolveDomain(requested string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(requested))
	if lower == "" {
		return "", "domain_required"
	}
	for _, d := range s.domains {
		if strings.ToLower(d) == lower {
			return d, ""
		}
	}
	return "", "domain_not_allowed"
}

// CheckAliasResult is returned by HandleCheckAlias.
type CheckAliasResult struct {
	Exists bool
	Alias  string
}

// HandleCheckAlias validates the alias and checks whether it exists in forwardemail.net.
// Returns errCode "alias_invalid", "alias_reserved", "domain_required", "domain_not_allowed", or "forwardemail_error" on failure.
func (s *ForwardService) HandleCheckAlias(ctx context.Context, alias, requestedDomain string) (CheckAliasResult, string) {
	domain, errCode := s.resolveDomain(requestedDomain)
	if errCode != "" {
		return CheckAliasResult{}, errCode
	}

	normalised, errCode := model.ValidateAlias(alias, domain, s.extraReserved)
	if errCode != "" {
		return CheckAliasResult{}, errCode
	}

	exists, err := s.feClient.AliasExists(ctx, domain, normalised)
	if err != nil {
		slog.ErrorContext(ctx, "check_alias: forwardemail error", "alias", normalised, "error", err)
		return CheckAliasResult{}, "forwardemail_error"
	}

	return CheckAliasResult{Exists: exists, Alias: normalised}, ""
}

// SetTargetResult is returned by HandleSetTarget.
type SetTargetResult struct {
	Alias       string
	TargetEmail string
	UpdatedAt   time.Time
}

// HandleSetTarget creates or updates the forwarding routing for the caller's alias on the given domain.
// Returns errCode "unauthorized", "not_found", "target_email_invalid", "domain_required", "domain_not_allowed", or "forwardemail_error" on failure.
func (s *ForwardService) HandleSetTarget(ctx context.Context, authToken, requestedDomain, targetEmail string) (SetTargetResult, string) {
	domain, errCode := s.resolveDomain(requestedDomain)
	if errCode != "" {
		return SetTargetResult{}, errCode
	}

	// Verify JWT signature locally (defense in depth); auth-service re-validates below.
	claims, err := s.jwtCfg.Verify(ctx, authToken)
	if err != nil {
		slog.WarnContext(ctx, "set_target: JWT verification failed", "error", err)
		return SetTargetResult{}, "unauthorized"
	}
	sub := claims.Subject

	authCtx, authCancel := context.WithTimeout(ctx, s.authServiceTimeout)
	defer authCancel()

	alias, err := s.authClient.GetAliasForDomain(authCtx, authToken, domain)
	if err != nil {
		if errors.Is(err, authservice.ErrNoAliasForDomain) {
			return SetTargetResult{}, "not_found"
		}
		slog.ErrorContext(ctx, "set_target: auth-service error", "error", err)
		return SetTargetResult{}, "unauthorized"
	}

	targetEmail = strings.TrimSpace(targetEmail)
	if targetEmail == "" {
		return SetTargetResult{}, "target_email_invalid"
	}
	if _, err := mail.ParseAddress(targetEmail); err != nil {
		return SetTargetResult{}, "target_email_invalid"
	}

	exists, err := s.feClient.AliasExists(ctx, domain, alias)
	if err != nil {
		slog.ErrorContext(ctx, "set_target: check alias existence failed", "alias", alias, "error", err)
		return SetTargetResult{}, "forwardemail_error"
	}

	var updatedAt time.Time

	if !exists {
		created, err := s.feClient.CreateAlias(ctx, domain, &femail.CreateAliasRequest{
			Name:       alias,
			Recipients: []string{targetEmail},
			Labels:     []string{fmt.Sprintf("lfid:%s", sub)},
			IsEnabled:  true,
		})
		if err != nil {
			slog.ErrorContext(ctx, "set_target: create alias failed", "alias", alias, "error", err)
			return SetTargetResult{}, "forwardemail_error"
		}
		updatedAt = parseUpdatedAt(ctx, created.UpdatedAt)
		slog.InfoContext(ctx, "set_target: alias created", "alias", alias, "domain", domain)
	} else {
		updated, err := s.feClient.UpdateAlias(ctx, domain, alias, &femail.UpdateAliasRequest{
			Recipients: []string{targetEmail},
			IsEnabled:  true,
		})
		if err != nil {
			slog.ErrorContext(ctx, "set_target: update alias failed", "alias", alias, "error", err)
			return SetTargetResult{}, "forwardemail_error"
		}
		updatedAt = parseUpdatedAt(ctx, updated.UpdatedAt)
		slog.InfoContext(ctx, "set_target: alias updated", "alias", alias, "domain", domain)
	}

	return SetTargetResult{
		Alias:       alias,
		TargetEmail: targetEmail,
		UpdatedAt:   updatedAt,
	}, ""
}

// GetForwardResult is returned by HandleGetForward.
type GetForwardResult struct {
	Found       bool
	Alias       string
	TargetEmail string
}

// HandleGetForward returns the current forwarding routing for the caller's alias on the given domain.
// Returns errCode "unauthorized", "domain_required", "domain_not_allowed", or "forwardemail_error" on failure.
// Returns GetForwardResult{Found: false} when the user has no identity for the requested domain.
func (s *ForwardService) HandleGetForward(ctx context.Context, authToken, requestedDomain string) (GetForwardResult, string) {
	domain, errCode := s.resolveDomain(requestedDomain)
	if errCode != "" {
		return GetForwardResult{}, errCode
	}

	// Verify JWT signature locally (defense in depth); auth-service re-validates below.
	if _, err := s.jwtCfg.Verify(ctx, authToken); err != nil {
		slog.WarnContext(ctx, "get_forward: JWT verification failed", "error", err)
		return GetForwardResult{}, "unauthorized"
	}

	authCtx, authCancel := context.WithTimeout(ctx, s.authServiceTimeout)
	defer authCancel()

	alias, err := s.authClient.GetAliasForDomain(authCtx, authToken, domain)
	if err != nil {
		if errors.Is(err, authservice.ErrNoAliasForDomain) {
			return GetForwardResult{Found: false}, ""
		}
		slog.ErrorContext(ctx, "get_forward: auth-service error", "error", err)
		return GetForwardResult{}, "unauthorized"
	}

	aliasObj, err := s.feClient.GetAlias(ctx, domain, alias)
	if err != nil {
		if errors.Is(err, femail.ErrNotFound) {
			return GetForwardResult{Found: false}, ""
		}
		slog.ErrorContext(ctx, "get_forward: forwardemail error", "alias", alias, "error", err)
		return GetForwardResult{}, "forwardemail_error"
	}

	target := ""
	if len(aliasObj.Recipients) > 0 {
		target = aliasObj.Recipients[0]
	}

	return GetForwardResult{
		Found:       true,
		Alias:       alias,
		TargetEmail: target,
	}, ""
}

func parseUpdatedAt(ctx context.Context, s string) time.Time {
	if s == "" {
		slog.WarnContext(ctx, "parseUpdatedAt: empty updated_at from forwardemail.net, using current time")
		return time.Now().UTC()
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.999Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	slog.WarnContext(ctx, "parseUpdatedAt: failed to parse updated_at from forwardemail.net, using current time", "value", s)
	return time.Now().UTC()
}
