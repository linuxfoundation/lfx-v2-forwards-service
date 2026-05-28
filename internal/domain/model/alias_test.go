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
