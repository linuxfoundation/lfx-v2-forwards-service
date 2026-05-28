// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// AuthServiceClient is the port for calling lfx-v2-auth-service.
type AuthServiceClient interface {
	// GetAliasForDomain returns the alias local part (e.g. "johndoe") for the user
	// identified by authToken on the given domain. Returns authservice.ErrNoAliasForDomain
	// if the user has no identity for that domain.
	GetAliasForDomain(ctx context.Context, authToken, domain string) (string, error)
}
