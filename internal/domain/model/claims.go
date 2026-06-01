// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "time"

// Claims is the domain representation of the verified JWT claims used by the
// service. The jwt adapter populates it from the parsed token at the boundary.
type Claims struct {
	Subject   string
	Email     string
	ExpiresAt *time.Time
	IssuedAt  *time.Time
	NotBefore *time.Time
	Issuer    string
	Audience  string
	Scope     string
	Raw       map[string]any
}
