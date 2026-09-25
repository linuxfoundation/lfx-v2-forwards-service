// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model_test

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
)

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		name          string
		alias         string
		domain        string
		extraReserved []string
		wantNorm      string
		wantCode      string
	}{
		{name: "valid simple", alias: "johndoe", wantNorm: "johndoe"},
		{name: "valid uppercased", alias: "JohnDoe", wantNorm: "johndoe"},
		{name: "valid with hyphen", alias: "john-doe", wantNorm: "john-doe"},
		{name: "valid with dot", alias: "john.doe", wantNorm: "john.doe"},
		{name: "valid with numbers", alias: "user123", wantNorm: "user123"},
		{name: "empty", alias: "", wantCode: "alias_invalid"},
		{name: "too long", alias: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantCode: "alias_invalid"}, // 65 chars
		{name: "64 chars valid", alias: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantNorm: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, // 64 chars
		{name: "banned char slash", alias: "john/doe", wantCode: "alias_invalid"},
		{name: "banned char asterisk", alias: "john*doe", wantCode: "alias_invalid"},
		{name: "banned char at", alias: "john@doe", wantCode: "alias_invalid"},
		{name: "banned char space", alias: "john doe", wantCode: "alias_invalid"},
		{name: "reserved postmaster", alias: "postmaster", wantCode: "alias_reserved"},
		{name: "reserved admin uppercase", alias: "ADMIN", wantCode: "alias_reserved"},
		{name: "reserved abuse", alias: "abuse", wantCode: "alias_reserved"},
		{name: "reserved linux", alias: "linux", wantCode: "alias_reserved"},
		{name: "extra reserved", alias: "custom", extraReserved: []string{"custom"}, wantCode: "alias_reserved"},
		{name: "extra reserved case insensitive", alias: "Custom", extraReserved: []string{"CUSTOM"}, wantCode: "alias_reserved"},
		{name: "extra reserved not matching", alias: "custom2", extraReserved: []string{"custom"}, wantNorm: "custom2"},
		{name: "trim whitespace", alias: "  johndoe  ", wantNorm: "johndoe"},
		// Same rules apply across managed domains.
		{name: "valid on linuxfoundation.org", alias: "johndoe", domain: "linuxfoundation.org", wantNorm: "johndoe"},
		{name: "reserved postmaster on linuxfoundation.org", alias: "postmaster", domain: "linuxfoundation.org", wantCode: "alias_reserved"},
		{name: "banned char on linuxfoundation.org", alias: "john/doe", domain: "linuxfoundation.org", wantCode: "alias_invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := tt.domain
			if domain == "" {
				domain = "linux.com"
			}
			gotNorm, gotCode := model.ValidateAlias(tt.alias, domain, tt.extraReserved)
			if gotNorm != tt.wantNorm {
				t.Errorf("normalised = %q, want %q", gotNorm, tt.wantNorm)
			}
			if gotCode != tt.wantCode {
				t.Errorf("errCode = %q, want %q", gotCode, tt.wantCode)
			}
		})
	}
}

// reservedAliasCanary is the expected set of reserved local parts, mirroring the
// list in lfx-v2-auth-service. It is duplicated here on purpose: the two services
// share no code, so this test is the drift guard. If you change the reserved list
// in alias.go, update this slice too — and coordinate the same change in
// lfx-v2-auth-service, or a name reserved there becomes claimable here.
var reservedAliasCanary = []string{
	"postmaster", "abuse", "hostmaster", "admin", "administrator",
	"noreply", "no-reply", "root", "mailer-daemon", "linux",
	"linuxfoundation", "lf", "security", "support", "info",
	"webmaster", "ops", "devops", "itx-system",
}

// TestReservedAliasesRejected asserts every canonical reserved name is rejected by
// ValidateAlias. Removing a name from the reserved set in alias.go (without updating
// this canary) fails the test, making reserved-list drift visible in the diff.
func TestReservedAliasesRejected(t *testing.T) {
	for _, name := range reservedAliasCanary {
		t.Run(name, func(t *testing.T) {
			if _, code := model.ValidateAlias(name, "linux.com", nil); code != "alias_reserved" {
				t.Errorf("ValidateAlias(%q) code = %q, want %q", name, code, "alias_reserved")
			}
		})
	}
}
