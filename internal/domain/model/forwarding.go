// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

// Alias is the domain representation of a forwardemail.net alias routing record.
// Infrastructure adapters convert their wire shapes to and from this type at the
// boundary so the domain and service layers stay free of infrastructure imports.
type Alias struct {
	// Name is the local part of the alias (e.g. "johndoe").
	Name string
	// Recipients are the addresses mail to the alias is forwarded to.
	Recipients []string
	// UpdatedAt is the provider's last-write timestamp, as returned on the wire.
	UpdatedAt string
}

// CreateAliasRequest is the domain request to create an alias routing.
type CreateAliasRequest struct {
	Name       string
	Recipients []string
	Labels     []string
	IsEnabled  bool
}

// UpdateAliasRequest is the domain request to replace the recipients on an alias.
type UpdateAliasRequest struct {
	Recipients []string
	IsEnabled  bool
}
