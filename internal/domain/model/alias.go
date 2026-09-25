// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"net/mail"
	"strings"
)

// reservedLocalParts is the canonical set of local parts that may not be claimed
// as forwarding aliases on any managed domain. Matches the list in
// lfx-v2-auth-service/internal/domain/model/alias.go.
var reservedLocalParts = map[string]struct{}{
	"postmaster":      {},
	"abuse":           {},
	"hostmaster":      {},
	"admin":           {},
	"administrator":   {},
	"noreply":         {},
	"no-reply":        {},
	"root":            {},
	"mailer-daemon":   {},
	"linux":           {},
	"linuxfoundation": {},
	"lf":              {},
	"security":        {},
	"support":         {},
	"info":            {},
	"webmaster":       {},
	"ops":             {},
	"devops":          {},
	"itx-system":      {},
}

// bannedAliasChars are characters disallowed in an alias local part.
// Ported from itx-service-forwards/forwards.go:309-327. The double-quote prevents
// RFC 5322 quoted local-part bypass (e.g. `"admin"@linux.com`).
const bannedAliasChars = `/*$^:()<>[];@\, "`

// ValidateAlias normalises alias (lowercases + trims) against the given domain
// and returns ("", "alias_invalid") or ("", "alias_reserved") on failure,
// or (normalised, "") on success.
// extraReserved is an optional slice of additional reserved names (from FORWARDS_RESERVED_NAMES).
// The reserved-name and banned-char sets are shared across all managed domains.
func ValidateAlias(alias, domain string, extraReserved []string) (string, string) {
	alias = strings.ToLower(strings.TrimSpace(alias))

	if len(alias) == 0 || len(alias) > 64 {
		return "", "alias_invalid"
	}

	if strings.ContainsAny(alias, bannedAliasChars) {
		return "", "alias_invalid"
	}

	// Ensure the address parses and that net/mail's canonical form matches the
	// input verbatim (prevents escapes / encoded forms that canonicalize differently).
	addr := alias + "@" + domain
	parsed, err := mail.ParseAddress(addr)
	if err != nil || !strings.EqualFold(parsed.Address, addr) {
		return "", "alias_invalid"
	}

	if _, reserved := reservedLocalParts[alias]; reserved {
		return "", "alias_reserved"
	}
	for _, extra := range extraReserved {
		if strings.EqualFold(alias, strings.TrimSpace(extra)) {
			return "", "alias_reserved"
		}
	}

	return alias, ""
}
