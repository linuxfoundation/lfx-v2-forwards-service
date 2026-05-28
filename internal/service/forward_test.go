// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"
)

func newTestService(domains []string) *ForwardService {
	return New(Config{Domains: domains})
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
