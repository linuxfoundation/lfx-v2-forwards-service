// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
)

// TokenVerifier is the port for verifying a JWT access token and returning its claims.
// The production implementation is *jwt.Config (backed by the Auth0 JWKS); tests can
// substitute a mock to exercise the authenticated handlers without a real key set.
type TokenVerifier interface {
	// Verify validates the token's signature and standard claims, returning the parsed claims.
	Verify(ctx context.Context, token string) (*model.Claims, error)
}
