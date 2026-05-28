// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package jwt provides JWT parsing and verification backed by lestrrat-go/jwx/v2.
// Ported from lfx-v2-auth-service/pkg/jwt/parser.go with auth-service-specific
// error types replaced by standard Go errors.
package jwt

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Claims represents the parsed JWT claims with commonly used fields.
type Claims struct {
	Subject   string         `json:"sub"`
	Email     string         `json:"email,omitempty"`
	ExpiresAt *time.Time     `json:"exp,omitempty"`
	IssuedAt  *time.Time     `json:"iat,omitempty"`
	NotBefore *time.Time     `json:"nbf,omitempty"`
	Issuer    string         `json:"iss,omitempty"`
	Audience  string         `json:"aud,omitempty"`
	Scope     string         `json:"scope,omitempty"`
	Raw       map[string]any `json:"-"`
}

// ParseOptions configures JWT parsing behavior.
type ParseOptions struct {
	RequireExpiration bool
	RequiredScopes    []string
	AllowBearerPrefix bool
	RequireSubject    bool
	VerifySignature   bool
	SigningKey        *rsa.PublicKey
	ExpectedIssuer    string
	ExpectedAudience  string
}

// DefaultParseOptions returns sensible defaults.
func DefaultParseOptions() *ParseOptions {
	return &ParseOptions{
		RequireExpiration: true,
		AllowBearerPrefix: true,
		RequireSubject:    true,
	}
}

// ParseVerified parses a JWT token with RSA signature verification.
func ParseVerified(ctx context.Context, tokenString string, opts *ParseOptions) (*Claims, error) {
	if opts == nil {
		opts = DefaultParseOptions()
	}

	cleanToken, err := cleanTokenString(tokenString, opts.AllowBearerPrefix)
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse([]byte(cleanToken), jwt.WithKey(jwa.RS256, opts.SigningKey))
	if err != nil {
		return nil, fmt.Errorf("JWT signature verification failed: %w", err)
	}

	claims, err := extractClaimsFromJWT(token)
	if err != nil {
		return nil, err
	}

	if opts.ExpectedIssuer != "" {
		if claims.Issuer != opts.ExpectedIssuer {
			return nil, fmt.Errorf("invalid issuer %q, expected %q", claims.Issuer, opts.ExpectedIssuer)
		}
	}

	if opts.ExpectedAudience != "" {
		if claims.Audience != opts.ExpectedAudience {
			return nil, fmt.Errorf("invalid audience")
		}
	}

	if opts.RequireExpiration {
		if err := validateExpiration(claims); err != nil {
			return nil, err
		}
	}

	if opts.RequireSubject {
		if strings.TrimSpace(claims.Subject) == "" {
			return nil, fmt.Errorf("missing or invalid 'sub' claim in token")
		}
	}

	if len(opts.RequiredScopes) > 0 {
		if err := validateScopes(claims, opts.RequiredScopes); err != nil {
			return nil, err
		}
	}

	slog.DebugContext(ctx, "JWT parsed and verified successfully",
		"sub", claims.Subject,
		"issuer", claims.Issuer,
		"expires_at", claims.ExpiresAt,
	)

	return claims, nil
}

// ParseUnverified parses a JWT token without signature verification.
// Useful for extracting claims when the token is validated by a downstream service.
func ParseUnverified(ctx context.Context, tokenString string, opts *ParseOptions) (*Claims, error) {
	if opts == nil {
		opts = DefaultParseOptions()
	}

	cleanToken, err := cleanTokenString(tokenString, opts.AllowBearerPrefix)
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse([]byte(cleanToken), jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	claims, err := extractClaimsFromJWT(token)
	if err != nil {
		return nil, err
	}

	if opts.RequireExpiration {
		if err := validateExpiration(claims); err != nil {
			return nil, err
		}
	}

	if opts.RequireSubject {
		if strings.TrimSpace(claims.Subject) == "" {
			return nil, fmt.Errorf("missing or invalid 'sub' claim in token")
		}
	}

	slog.DebugContext(ctx, "JWT parsed (unverified)", "sub", claims.Subject)
	return claims, nil
}

// ExtractSubject extracts only the 'sub' claim without signature verification.
func ExtractSubject(ctx context.Context, tokenString string) (string, error) {
	opts := &ParseOptions{
		RequireExpiration: false,
		AllowBearerPrefix: true,
		RequireSubject:    false,
	}
	claims, err := ParseUnverified(ctx, tokenString, opts)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return "", fmt.Errorf("missing or invalid 'sub' claim in token")
	}
	return claims.Subject, nil
}

// Config holds a loaded JWKS public key and the expected issuer/audience.
type Config struct {
	PublicKey        *rsa.PublicKey
	ExpectedIssuer   string
	ExpectedAudience string
}

// NewConfigFromJWKS fetches the JWKS from the Auth0 domain and returns a Config.
func NewConfigFromJWKS(ctx context.Context, auth0Domain, audience string) (*Config, error) {
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks.json", auth0Domain)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build JWKS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Use string `json:"use,omitempty"`
			Kid string `json:"kid,omitempty"`
			Alg string `json:"alg,omitempty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}

	for _, key := range jwks.Keys {
		if key.Kty != "RSA" || (key.Use != "sig" && key.Use != "") {
			continue
		}
		jwkData, err := json.Marshal(key)
		if err != nil {
			continue
		}
		pubKey, err := loadRSAPublicKeyFromJWK(jwkData)
		if err != nil {
			return nil, fmt.Errorf("load RSA public key from JWK: %w", err)
		}

		expectedIssuer := fmt.Sprintf("https://%s/", auth0Domain)
		if audience == "" {
			audience = fmt.Sprintf("https://%s/api/v2/", auth0Domain)
		}

		slog.InfoContext(ctx, "JWT verification configured",
			"issuer", expectedIssuer,
			"audience", audience,
			"key_id", key.Kid,
		)

		return &Config{
			PublicKey:        pubKey,
			ExpectedIssuer:   expectedIssuer,
			ExpectedAudience: audience,
		}, nil
	}

	return nil, fmt.Errorf("no suitable RSA signing key found in JWKS")
}

// Verify validates a JWT token using this Config and returns the claims.
func (c *Config) Verify(ctx context.Context, token string) (*Claims, error) {
	return ParseVerified(ctx, token, &ParseOptions{
		RequireExpiration: true,
		AllowBearerPrefix: true,
		RequireSubject:    true,
		VerifySignature:   true,
		SigningKey:        c.PublicKey,
		ExpectedIssuer:    c.ExpectedIssuer,
		ExpectedAudience:  c.ExpectedAudience,
	})
}

func cleanTokenString(tokenString string, allowBearer bool) (string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", fmt.Errorf("token is required")
	}
	clean := strings.TrimSpace(tokenString)
	if allowBearer {
		parts := strings.Fields(tokenString)
		if len(parts) > 1 && strings.EqualFold(parts[0], "Bearer") {
			clean = strings.Join(parts[1:], " ")
		}
	}
	return clean, nil
}

func extractClaimsFromJWT(token jwt.Token) (*Claims, error) {
	claims := &Claims{Raw: make(map[string]any)}

	claims.Subject = token.Subject()
	claims.Issuer = token.Issuer()

	if audience := token.Audience(); len(audience) > 0 {
		claims.Audience = audience[0]
	}

	if email, ok := token.Get("email"); ok {
		if s, ok := email.(string); ok {
			claims.Email = s
		}
	}
	if scope, ok := token.Get("scope"); ok {
		if s, ok := scope.(string); ok {
			claims.Scope = s
		}
	}

	if exp := token.Expiration(); !exp.IsZero() {
		claims.ExpiresAt = &exp
	}
	if iat := token.IssuedAt(); !iat.IsZero() {
		claims.IssuedAt = &iat
	}
	if nbf := token.NotBefore(); !nbf.IsZero() {
		claims.NotBefore = &nbf
	}

	maps.Copy(claims.Raw, token.PrivateClaims())
	return claims, nil
}

func validateExpiration(claims *Claims) error {
	if claims.ExpiresAt == nil {
		return fmt.Errorf("missing 'exp' claim in token")
	}
	if time.Now().After(*claims.ExpiresAt) {
		return fmt.Errorf("token has expired at %v", *claims.ExpiresAt)
	}
	return nil
}

func validateScopes(claims *Claims, required []string) error {
	if claims.Scope == "" {
		return fmt.Errorf("missing 'scope' claim in token")
	}
	tokenScopes := strings.Fields(claims.Scope)
	for _, s := range required {
		if !slices.Contains(tokenScopes, s) {
			return fmt.Errorf("missing required scope %q", s)
		}
	}
	return nil
}

func loadRSAPublicKeyFromJWK(jwkData []byte) (*rsa.PublicKey, error) {
	key, err := jwk.ParseKey(jwkData)
	if err != nil {
		return nil, fmt.Errorf("parse JWK: %w", err)
	}
	var rsaKey rsa.PublicKey
	if err := key.Raw(&rsaKey); err != nil {
		return nil, fmt.Errorf("get RSA public key from JWK: %w", err)
	}
	return &rsaKey, nil
}
