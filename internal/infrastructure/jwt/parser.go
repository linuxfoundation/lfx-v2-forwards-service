// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package jwt provides JWT parsing and verification backed by lestrrat-go/jwx/v2.
// Ported from lfx-v2-auth-service/pkg/jwt/parser.go with auth-service-specific
// error types replaced by standard Go errors.
package jwt

import (
	"context"
	"crypto/rsa"
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

// Config holds a cached JWKS key set and the expected issuer/audience.
// The key set includes all RSA signing keys from the Auth0 JWKS endpoint;
// jwt.Parse selects the matching key by `kid` header automatically.
type Config struct {
	keySet           jwk.Set
	ExpectedIssuer   string
	ExpectedAudience string
}

// NewConfigFromJWKS fetches the JWKS from the Auth0 domain and returns a Config.
// Uses a dedicated HTTP client with a 10-second timeout to avoid hanging startup.
// All RSA signing keys in the response are retained so that key rotation is tolerated.
func NewConfigFromJWKS(ctx context.Context, auth0Domain, audience string) (*Config, error) {
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks.json", auth0Domain)

	httpClient := &http.Client{Timeout: 10 * time.Second}

	keySet, err := jwk.Fetch(ctx, jwksURL, jwk.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}

	if keySet.Len() == 0 {
		return nil, fmt.Errorf("no keys found in JWKS at %s", jwksURL)
	}

	expectedIssuer := fmt.Sprintf("https://%s/", auth0Domain)
	if audience == "" {
		audience = fmt.Sprintf("https://%s/api/v2/", auth0Domain)
	}

	slog.InfoContext(ctx, "JWT verification configured",
		"issuer", expectedIssuer,
		"audience", audience,
		"key_count", keySet.Len(),
	)

	return &Config{
		keySet:           keySet,
		ExpectedIssuer:   expectedIssuer,
		ExpectedAudience: audience,
	}, nil
}

// Verify validates a JWT token using this Config and returns the claims.
// Key selection is kid-aware: jwt.Parse matches the token's `kid` header against
// the key set so that key rotation does not cause valid tokens to be rejected.
func (c *Config) Verify(ctx context.Context, token string) (*Claims, error) {
	cleanToken, err := cleanTokenString(token, true)
	if err != nil {
		return nil, err
	}

	tok, err := jwt.Parse([]byte(cleanToken), jwt.WithKeySet(c.keySet), jwt.WithValidate(true))
	if err != nil {
		return nil, fmt.Errorf("JWT signature verification failed: %w", err)
	}

	claims, err := extractClaimsFromJWT(tok)
	if err != nil {
		return nil, err
	}

	if c.ExpectedIssuer != "" && claims.Issuer != c.ExpectedIssuer {
		return nil, fmt.Errorf("invalid issuer %q, expected %q", claims.Issuer, c.ExpectedIssuer)
	}
	if c.ExpectedAudience != "" && claims.Audience != c.ExpectedAudience {
		return nil, fmt.Errorf("invalid audience")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, fmt.Errorf("missing or invalid 'sub' claim in token")
	}

	slog.DebugContext(ctx, "JWT parsed and verified successfully",
		"sub", claims.Subject,
		"issuer", claims.Issuer,
		"expires_at", claims.ExpiresAt,
	)

	return claims, nil
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
