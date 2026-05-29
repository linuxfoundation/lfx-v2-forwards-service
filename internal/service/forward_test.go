// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/authservice"
	femail "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/forwardemail"
	jwtpkg "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/jwt"
)

// mockVerifier is a port.TokenVerifier whose result is configured per test.
type mockVerifier struct {
	claims *jwtpkg.Claims
	err    error
}

func (m *mockVerifier) Verify(_ context.Context, _ string) (*jwtpkg.Claims, error) {
	return m.claims, m.err
}

// mockAuthClient is a port.AuthServiceClient whose result is configured per test.
type mockAuthClient struct {
	alias string
	err   error
}

func (m *mockAuthClient) GetAliasForDomain(_ context.Context, _, _ string) (string, error) {
	return m.alias, m.err
}

// mockFEClient is a port.ForwardEmailProvider whose per-method results are configured per test.
type mockFEClient struct {
	getAlias    *femail.Alias
	getAliasErr error

	exists    bool
	existsErr error

	created   *femail.Alias
	createErr error

	updated   *femail.Alias
	updateErr error
}

func (m *mockFEClient) GetAlias(_ context.Context, _, _ string) (*femail.Alias, error) {
	return m.getAlias, m.getAliasErr
}

func (m *mockFEClient) AliasExists(_ context.Context, _, _ string) (bool, error) {
	return m.exists, m.existsErr
}

func (m *mockFEClient) CreateAlias(_ context.Context, _ string, _ *femail.CreateAliasRequest) (*femail.Alias, error) {
	return m.created, m.createErr
}

func (m *mockFEClient) UpdateAlias(_ context.Context, _, _ string, _ *femail.UpdateAliasRequest) (*femail.Alias, error) {
	return m.updated, m.updateErr
}

// newTestService builds a service with a benign (non-nil) verifier and no clients,
// for tests that only exercise validation paths returning before any client call.
func newTestService(domains []string) *ForwardService {
	s, err := New(Config{Domains: domains, JWTConfig: &mockVerifier{}})
	if err != nil {
		panic(err)
	}
	return s
}

func TestResolveDomain(t *testing.T) {
	s := newTestService([]string{"linux.com", "linuxfoundation.org"})

	tests := []struct {
		name      string
		requested string
		wantDom   string
		wantErr   string
	}{
		{"empty returns domain_required", "", "", "domain_required"},
		{"whitespace returns domain_required", "   ", "", "domain_required"},
		{"exact match", "linux.com", "linux.com", ""},
		{"case-insensitive match", "LINUX.COM", "linux.com", ""},
		{"second domain", "linuxfoundation.org", "linuxfoundation.org", ""},
		{"unknown domain returns domain_not_allowed", "hurrdurr.org", "", "domain_not_allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dom, errCode := s.resolveDomain(tt.requested)
			if dom != tt.wantDom || errCode != tt.wantErr {
				t.Errorf("resolveDomain(%q) = (%q, %q), want (%q, %q)",
					tt.requested, dom, errCode, tt.wantDom, tt.wantErr)
			}
		})
	}
}

func TestHandleCheckAlias_DomainRequired(t *testing.T) {
	s := newTestService([]string{"linux.com"})
	_, errCode := s.HandleCheckAlias(context.Background(), "johndoe", "")
	if errCode != "domain_required" {
		t.Errorf("expected domain_required, got %q", errCode)
	}
}

func TestHandleCheckAlias_DomainNotAllowed(t *testing.T) {
	s := newTestService([]string{"linux.com"})
	_, errCode := s.HandleCheckAlias(context.Background(), "johndoe", "hurrdurr.org")
	if errCode != "domain_not_allowed" {
		t.Errorf("expected domain_not_allowed, got %q", errCode)
	}
}

func TestNew_EmptyDomains(t *testing.T) {
	_, err := New(Config{Domains: []string{}})
	if err == nil {
		t.Error("expected error for empty domains, got nil")
	}
}

func TestNew_NilDomains(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Error("expected error for nil domains, got nil")
	}
}

func TestNew_NilJWTConfig(t *testing.T) {
	_, err := New(Config{Domains: []string{"linux.com"}})
	if err == nil {
		t.Error("expected error for nil JWTConfig, got nil")
	}
}

func TestHandleSetTarget(t *testing.T) {
	const domain = "linux.com"
	okClaims := &jwtpkg.Claims{Subject: "auth0|abc"}

	tests := []struct {
		name       string
		verifier   *mockVerifier
		auth       *mockAuthClient
		fe         *mockFEClient
		target     string
		wantErr    string
		wantAlias  string
		wantTarget string
	}{
		{
			name:     "jwt failure returns unauthorized",
			verifier: &mockVerifier{err: context.DeadlineExceeded},
			auth:     &mockAuthClient{},
			fe:       &mockFEClient{},
			target:   "me@example.com",
			wantErr:  "unauthorized",
		},
		{
			name:     "no alias for domain returns not_found",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{err: authservice.ErrNoAliasForDomain},
			fe:       &mockFEClient{},
			target:   "me@example.com",
			wantErr:  "not_found",
		},
		{
			name:     "auth-service error returns unauthorized",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{err: context.DeadlineExceeded},
			fe:       &mockFEClient{},
			target:   "me@example.com",
			wantErr:  "unauthorized",
		},
		{
			name:     "invalid target email returns target_email_invalid",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{alias: "johndoe"},
			fe:       &mockFEClient{},
			target:   "not-an-email",
			wantErr:  "target_email_invalid",
		},
		{
			name:     "forwardemail existence check error returns forwardemail_error",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{alias: "johndoe"},
			fe:       &mockFEClient{existsErr: context.DeadlineExceeded},
			target:   "me@example.com",
			wantErr:  "forwardemail_error",
		},
		{
			name:     "create error returns forwardemail_error",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{alias: "johndoe"},
			fe:       &mockFEClient{exists: false, createErr: context.DeadlineExceeded},
			target:   "me@example.com",
			wantErr:  "forwardemail_error",
		},
		{
			name:       "happy path creates alias",
			verifier:   &mockVerifier{claims: okClaims},
			auth:       &mockAuthClient{alias: "johndoe"},
			fe:         &mockFEClient{exists: false, created: &femail.Alias{UpdatedAt: "2026-01-02T15:04:05Z"}},
			target:     "me@example.com",
			wantAlias:  "johndoe",
			wantTarget: "me@example.com",
		},
		{
			name:       "happy path updates existing alias",
			verifier:   &mockVerifier{claims: okClaims},
			auth:       &mockAuthClient{alias: "johndoe"},
			fe:         &mockFEClient{exists: true, updated: &femail.Alias{UpdatedAt: "2026-01-02T15:04:05Z"}},
			target:     "me@example.com",
			wantAlias:  "johndoe",
			wantTarget: "me@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(Config{
				Domains:      []string{domain},
				JWTConfig:    tt.verifier,
				AuthClient:   tt.auth,
				FEmailClient: tt.fe,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			res, errCode := s.HandleSetTarget(context.Background(), "token", domain, tt.target)
			if errCode != tt.wantErr {
				t.Fatalf("errCode = %q, want %q", errCode, tt.wantErr)
			}
			if tt.wantErr != "" {
				return
			}
			if res.Alias != tt.wantAlias {
				t.Errorf("alias = %q, want %q", res.Alias, tt.wantAlias)
			}
			if res.TargetEmail != tt.wantTarget {
				t.Errorf("target = %q, want %q", res.TargetEmail, tt.wantTarget)
			}
		})
	}
}

func TestHandleGetForward(t *testing.T) {
	const domain = "linux.com"
	okClaims := &jwtpkg.Claims{Subject: "auth0|abc"}

	tests := []struct {
		name       string
		verifier   *mockVerifier
		auth       *mockAuthClient
		fe         *mockFEClient
		wantErr    string
		wantFound  bool
		wantAlias  string
		wantTarget string
	}{
		{
			name:     "jwt failure returns unauthorized",
			verifier: &mockVerifier{err: context.DeadlineExceeded},
			auth:     &mockAuthClient{},
			fe:       &mockFEClient{},
			wantErr:  "unauthorized",
		},
		{
			name:      "no alias for domain returns not found, no error",
			verifier:  &mockVerifier{claims: okClaims},
			auth:      &mockAuthClient{err: authservice.ErrNoAliasForDomain},
			fe:        &mockFEClient{},
			wantFound: false,
		},
		{
			name:     "auth-service error returns unauthorized",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{err: context.DeadlineExceeded},
			fe:       &mockFEClient{},
			wantErr:  "unauthorized",
		},
		{
			name:      "alias absent in forwardemail returns not found, no error",
			verifier:  &mockVerifier{claims: okClaims},
			auth:      &mockAuthClient{alias: "johndoe"},
			fe:        &mockFEClient{getAliasErr: femail.ErrNotFound},
			wantFound: false,
		},
		{
			name:     "forwardemail error returns forwardemail_error",
			verifier: &mockVerifier{claims: okClaims},
			auth:     &mockAuthClient{alias: "johndoe"},
			fe:       &mockFEClient{getAliasErr: context.DeadlineExceeded},
			wantErr:  "forwardemail_error",
		},
		{
			name:       "happy path returns current target",
			verifier:   &mockVerifier{claims: okClaims},
			auth:       &mockAuthClient{alias: "johndoe"},
			fe:         &mockFEClient{getAlias: &femail.Alias{Recipients: []string{"me@example.com"}}},
			wantFound:  true,
			wantAlias:  "johndoe",
			wantTarget: "me@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(Config{
				Domains:      []string{domain},
				JWTConfig:    tt.verifier,
				AuthClient:   tt.auth,
				FEmailClient: tt.fe,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			res, errCode := s.HandleGetForward(context.Background(), "token", domain)
			if errCode != tt.wantErr {
				t.Fatalf("errCode = %q, want %q", errCode, tt.wantErr)
			}
			if tt.wantErr != "" {
				return
			}
			if res.Found != tt.wantFound {
				t.Errorf("found = %v, want %v", res.Found, tt.wantFound)
			}
			if res.Alias != tt.wantAlias {
				t.Errorf("alias = %q, want %q", res.Alias, tt.wantAlias)
			}
			if res.TargetEmail != tt.wantTarget {
				t.Errorf("target = %q, want %q", res.TargetEmail, tt.wantTarget)
			}
		})
	}
}

func TestParseUpdatedAt(t *testing.T) {
	ctx := context.Background()
	const rfc3339Input = "2026-01-02T15:04:05Z"
	expected, _ := time.Parse(time.RFC3339, rfc3339Input)

	t.Run("rfc3339 parses to exact value", func(t *testing.T) {
		result := parseUpdatedAt(ctx, rfc3339Input)
		if !result.Equal(expected) {
			t.Errorf("got %v, want %v", result, expected)
		}
	})

	t.Run("empty falls back to near-now", func(t *testing.T) {
		before := time.Now()
		result := parseUpdatedAt(ctx, "")
		if result.Before(before) || time.Since(result) > 5*time.Second {
			t.Errorf("expected time close to now, got %v", result)
		}
	})

	t.Run("unparseable falls back to near-now", func(t *testing.T) {
		before := time.Now()
		result := parseUpdatedAt(ctx, "not-a-date")
		if result.Before(before) || time.Since(result) > 5*time.Second {
			t.Errorf("expected time close to now, got %v", result)
		}
	})
}
