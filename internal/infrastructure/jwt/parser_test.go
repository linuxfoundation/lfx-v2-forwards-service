// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"slices"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// newTestKey generates a throwaway RSA key pair for signing test tokens.
func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}
	return key
}

// signToken builds and signs a JWT with the given audiences and subject.
func signToken(t *testing.T, key *rsa.PrivateKey, audiences []string, subject string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Audience(audiences).
		Subject(subject).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func TestParseVerified_AudienceMembership(t *testing.T) {
	key := newTestKey(t)
	ctx := context.Background()
	const apiAud = "https://api.example.com/"

	tests := []struct {
		name             string
		tokenAudiences   []string
		expectedAudience string
		wantErr          bool
	}{
		{
			name:             "api audience at index 0 (single)",
			tokenAudiences:   []string{apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "api audience at index 1 (multi-audience token)",
			tokenAudiences:   []string{"https://tenant.auth0.com/userinfo", apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "api audience at index 2",
			tokenAudiences:   []string{"https://other1/", "https://other2/", apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "expected audience absent from token",
			tokenAudiences:   []string{"https://other/", "https://tenant.auth0.com/userinfo"},
			expectedAudience: apiAud,
			wantErr:          true,
		},
		{
			name:             "empty ExpectedAudience skips check",
			tokenAudiences:   []string{"https://anything/"},
			expectedAudience: "",
			wantErr:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenStr := signToken(t, key, tc.tokenAudiences, "auth0|testuser")
			opts := &ParseOptions{
				RequireExpiration: true,
				AllowBearerPrefix: false,
				RequireSubject:    true,
				VerifySignature:   true,
				SigningKey:        &key.PublicKey,
				ExpectedAudience:  tc.expectedAudience,
			}
			claims, err := ParseVerified(ctx, tokenStr, opts)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got claims: %+v", claims)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.expectedAudience != "" && !slices.Contains(claims.Audience, tc.expectedAudience) {
				t.Errorf("claims.Audience %v does not contain %q", claims.Audience, tc.expectedAudience)
			}
		})
	}
}

// newTestKeySet returns a JWKS containing the public half of key, tagged with kid
// and RS256, matching the shape Config.Verify expects from the Auth0 JWKS endpoint.
func newTestKeySet(t *testing.T, key *rsa.PrivateKey, kid string) jwk.Set {
	t.Helper()
	pub, err := jwk.FromRaw(&key.PublicKey)
	if err != nil {
		t.Fatalf("build public jwk: %v", err)
	}
	if err := pub.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatalf("set kid: %v", err)
	}
	if err := pub.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		t.Fatalf("set alg: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("add key to set: %v", err)
	}
	return set
}

// signTokenWithKID signs a token whose JWS header carries kid, so that
// jwt.WithKeySet (which requires a matching kid by default in jwx v2) can
// select the verifying key — mirroring how Config.Verify runs in production.
func signTokenWithKID(t *testing.T, key *rsa.PrivateKey, audiences []string, subject, kid string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Audience(audiences).
		Subject(subject).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signKey, err := jwk.FromRaw(key)
	if err != nil {
		t.Fatalf("build signing jwk: %v", err)
	}
	if err := signKey.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatalf("set signing kid: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, signKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

// TestConfigVerify_AudienceMembership exercises the production JWKS-backed path
// (Config.Verify), which is what every authenticated request actually runs and
// is the path that rejected legitimate multi-audience tokens in LFXV2-2115.
// TestParseVerified_AudienceMembership covers the RSA-key path but never calls
// Verify, so this test guards against a regression of the slices.Contains fix.
func TestConfigVerify_AudienceMembership(t *testing.T) {
	key := newTestKey(t)
	ctx := context.Background()
	const apiAud = "https://api.example.com/"
	const kid = "test-key-1"

	set := newTestKeySet(t, key, kid)

	tests := []struct {
		name             string
		tokenAudiences   []string
		expectedAudience string
		wantErr          bool
	}{
		{
			name:             "api audience at index 0 (single)",
			tokenAudiences:   []string{apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "api audience at index 1 (multi-audience token)",
			tokenAudiences:   []string{"https://tenant.auth0.com/userinfo", apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "api audience at index 2",
			tokenAudiences:   []string{"https://other1/", "https://other2/", apiAud},
			expectedAudience: apiAud,
			wantErr:          false,
		},
		{
			name:             "expected audience absent from token",
			tokenAudiences:   []string{"https://other/", "https://tenant.auth0.com/userinfo"},
			expectedAudience: apiAud,
			wantErr:          true,
		},
		{
			name:             "empty ExpectedAudience skips check",
			tokenAudiences:   []string{"https://anything/"},
			expectedAudience: "",
			wantErr:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				keySet:           set,
				ExpectedAudience: tc.expectedAudience,
			}
			tokenStr := signTokenWithKID(t, key, tc.tokenAudiences, "auth0|testuser", kid)
			claims, err := cfg.Verify(ctx, tokenStr)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got claims: %+v", claims)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.expectedAudience != "" && !slices.Contains(claims.Audience, tc.expectedAudience) {
				t.Errorf("claims.Audience %v does not contain %q", claims.Audience, tc.expectedAudience)
			}
		})
	}
}
