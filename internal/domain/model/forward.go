// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "time"

// Forward represents the forwarding routing for a @linux.com alias.
type Forward struct {
	// Alias is the local part of the alias (e.g. "johndoe" for "johndoe@linux.com").
	Alias string
	// TargetEmail is the address that mail to Alias@linux.com is forwarded to.
	TargetEmail string
	// UpdatedAt is when the routing was last written to forwardemail.net.
	UpdatedAt time.Time
}
