// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package forwardemail provides a REST client for the forwardemail.net API.
// Ported from itx-service-forwards/forwardemail/forwardemail.go with improvements:
// proper 404 → ErrNotFound surfacing and 429 retry with exponential backoff.
package forwardemail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultBaseURL = "https://api.forwardemail.net"

// Client is a forwardemail.net REST client.
type Client struct {
	http    *http.Client
	baseURL string
	token   string
}

// New creates a forwardemail.net client.
func New(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		baseURL: baseURL,
		token:   token,
	}
}

// wireAlias is the forwardemail.net alias object returned by the API.
//
// Note: the API returns the "domain" field as an object on GET but as a bare
// string (the domain id) on the PUT response. It is intentionally not modeled
// here — the decoder ignores the unknown key regardless of shape, which keeps
// UpdateAlias from failing to decode a response that otherwise applied.
type wireAlias struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Labels     []string `json:"labels"`
	Recipients []string `json:"recipients"`
	IsEnabled  bool     `json:"is_enabled"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// toModel converts the wire alias to the domain representation.
func (w *wireAlias) toModel() *model.Alias {
	return &model.Alias{
		Name:       w.Name,
		Recipients: w.Recipients,
		UpdatedAt:  w.UpdatedAt,
	}
}

// wireCreateAliasRequest is the body for POST /v1/domains/:domain/aliases.
type wireCreateAliasRequest struct {
	Name                     string   `json:"name"`
	Recipients               []string `json:"recipients"`
	Labels                   []string `json:"labels,omitempty"`
	HasRecipientVerification bool     `json:"has_recipient_verification"`
	IsEnabled                bool     `json:"is_enabled"`
}

// wireUpdateAliasRequest is the body for PUT /v1/domains/:domain/aliases/:alias.
type wireUpdateAliasRequest struct {
	Recipients []string `json:"recipients"`
	IsEnabled  bool     `json:"is_enabled"`
}

// apiError represents a forwardemail.net API error response.
type apiError struct {
	StatusCode int    `json:"statusCode"`
	Err        string `json:"error"`
	Message    string `json:"message"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("forwardemail API error %d: %s — %s", e.StatusCode, e.Err, e.Message)
}

// GetAlias fetches an alias by local part from the given domain.
// Returns model.ErrAliasNotFound if the API returns 404.
func (c *Client) GetAlias(ctx context.Context, domain, alias string) (*model.Alias, error) {
	uri := fmt.Sprintf("%s/v1/domains/%s/aliases/%s", c.baseURL, url.PathEscape(domain), url.PathEscape(alias))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	var result wireAlias
	if err := c.do(req, nil, &result); err != nil {
		return nil, err
	}
	return result.toModel(), nil
}

// AliasExists returns true if the alias exists in forwardemail.net, false if it
// returns 404. Any other error is propagated.
func (c *Client) AliasExists(ctx context.Context, domain, alias string) (bool, error) {
	_, err := c.GetAlias(ctx, domain, alias)
	if errors.Is(err, model.ErrAliasNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateAlias creates a new alias in the given domain.
func (c *Client) CreateAlias(ctx context.Context, domain string, body *model.CreateAliasRequest) (*model.Alias, error) {
	uri := fmt.Sprintf("%s/v1/domains/%s/aliases", c.baseURL, url.PathEscape(domain))

	data, err := json.Marshal(&wireCreateAliasRequest{
		Name:       body.Name,
		Recipients: body.Recipients,
		Labels:     body.Labels,
		IsEnabled:  body.IsEnabled,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var result wireAlias
	if err := c.do(req, data, &result); err != nil {
		return nil, err
	}
	return result.toModel(), nil
}

// UpdateAlias updates the recipients of an existing alias.
func (c *Client) UpdateAlias(ctx context.Context, domain, alias string, body *model.UpdateAliasRequest) (*model.Alias, error) {
	uri := fmt.Sprintf("%s/v1/domains/%s/aliases/%s", c.baseURL, url.PathEscape(domain), url.PathEscape(alias))

	data, err := json.Marshal(&wireUpdateAliasRequest{
		Recipients: body.Recipients,
		IsEnabled:  body.IsEnabled,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uri, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var result wireAlias
	if err := c.do(req, data, &result); err != nil {
		return nil, err
	}
	return result.toModel(), nil
}

// do executes an HTTP request with basic auth, JSON headers, and 429 retry backoff.
// bodyBytes must be the raw JSON body for POST/PUT/PATCH requests; it is used to
// rebuild the request body for each attempt since the reader is consumed on first use.
// On HTTP 404, it returns model.ErrAliasNotFound. On other non-2xx responses it returns an *apiError.
func (c *Client) do(req *http.Request, bodyBytes []byte, out interface{}) error {
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.token, "")
	if req.Method == http.MethodPost || req.Method == http.MethodPut || req.Method == http.MethodPatch {
		req.Header.Set("Content-Type", "application/json")
	}

	const maxRetries = 3
	backoff := 500 * time.Millisecond

	for attempt := range maxRetries {
		// Rebuild body for each attempt: the reader from the previous attempt is consumed.
		if len(bodyBytes) > 0 {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt == maxRetries-1 {
				return &apiError{StatusCode: http.StatusTooManyRequests, Err: "rate_limited", Message: "forwardemail.net rate limit exceeded"}
			}
			select {
			case <-req.Context().Done():
				return req.Context().Err()
			case <-time.After(backoff):
				backoff *= 2
				continue
			}
		}

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return model.ErrAliasNotFound
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var apiErr apiError
			_ = json.NewDecoder(resp.Body).Decode(&apiErr)
			_ = resp.Body.Close()
			apiErr.StatusCode = resp.StatusCode
			return &apiErr
		}

		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				_ = resp.Body.Close()
				return fmt.Errorf("decode response: %w", err)
			}
		}
		_ = resp.Body.Close()
		return nil
	}

	return fmt.Errorf("max retries exceeded")
}
