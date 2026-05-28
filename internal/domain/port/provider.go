// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	femail "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/forwardemail"
)

// ForwardEmailProvider is the port for interacting with the forwardemail.net API.
type ForwardEmailProvider interface {
	// GetAlias fetches the alias object. Returns forwardemail.ErrNotFound if absent.
	GetAlias(ctx context.Context, domain, alias string) (*femail.Alias, error)
	// AliasExists reports whether an alias exists in forwardemail.net.
	AliasExists(ctx context.Context, domain, alias string) (bool, error)
	// CreateAlias creates a new alias routing.
	CreateAlias(ctx context.Context, domain string, body *femail.CreateAliasRequest) (*femail.Alias, error)
	// UpdateAlias replaces the recipients on an existing alias.
	UpdateAlias(ctx context.Context, domain, alias string, body *femail.UpdateAliasRequest) (*femail.Alias, error)
}
