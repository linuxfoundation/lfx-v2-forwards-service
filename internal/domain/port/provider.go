// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
)

// ForwardEmailProvider is the port for interacting with the forwardemail.net API.
type ForwardEmailProvider interface {
	// GetAlias fetches the alias object. Returns model.ErrAliasNotFound if absent.
	GetAlias(ctx context.Context, domain, alias string) (*model.Alias, error)
	// AliasExists reports whether an alias exists in forwardemail.net.
	AliasExists(ctx context.Context, domain, alias string) (bool, error)
	// CreateAlias creates a new alias routing.
	CreateAlias(ctx context.Context, domain string, body *model.CreateAliasRequest) (*model.Alias, error)
	// UpdateAlias replaces the recipients on an existing alias.
	UpdateAlias(ctx context.Context, domain, alias string, body *model.UpdateAliasRequest) (*model.Alias, error)
}
