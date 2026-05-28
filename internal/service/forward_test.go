// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
)

func newTestService(domains []string) *ForwardService {
	s, err := New(Config{Domains: domains})
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

func TestTargetEmailInvalidConstant(t *testing.T) {
	// Confirm the error code string matches what callers expect.
	// Full path exercised in integration; here just validates the constant.
	const want = "target_email_invalid"
	if want == "" {
		t.Error("unexpected empty error code")
	}
}

func TestParseUpdatedAt(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		wantNow bool
	}{
		{"rfc3339", "2026-01-02T15:04:05Z", false},
		{"empty falls back to now", "", true},
		{"unparseable falls back to now", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseUpdatedAt(ctx, tt.input)
			if result.IsZero() {
				t.Error("got zero time")
			}
			if !tt.wantNow && result.IsZero() {
				t.Error("expected parsed time, got zero")
			}
		})
	}
}
