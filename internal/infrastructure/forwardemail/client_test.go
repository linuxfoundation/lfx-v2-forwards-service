// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package forwardemail_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-forwards-service/internal/domain/model"
	femail "github.com/linuxfoundation/lfx-v2-forwards-service/internal/infrastructure/forwardemail"
)

func newTestClient(srv *httptest.Server) *femail.Client {
	return femail.New("test-token", srv.URL)
}

// assertBasicAuth fails the test if the request does not carry the expected API
// token as the Basic-auth username, guarding against the client ever dropping it.
func assertBasicAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if user, _, ok := r.BasicAuth(); !ok || user != "test-token" {
		t.Errorf("expected Basic auth with token, got %q", r.Header.Get("Authorization"))
	}
}

func TestGetAlias_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r)
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"johndoe","recipients":["john@example.com"]}`))
	}))
	defer srv.Close()

	client := newTestClient(srv)
	alias, err := client.GetAlias(context.Background(), "linux.com", "johndoe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias.Name != "johndoe" {
		t.Errorf("alias.Name = %q, want %q", alias.Name, "johndoe")
	}
	if len(alias.Recipients) == 0 || alias.Recipients[0] != "john@example.com" {
		t.Errorf("unexpected recipients: %v", alias.Recipients)
	}
}

func TestGetAlias_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	_, err := client.GetAlias(context.Background(), "linux.com", "unknown")
	if !errors.Is(err, model.ErrAliasNotFound) {
		t.Errorf("expected ErrAliasNotFound, got %v", err)
	}
}

func TestAliasExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	exists, err := client.AliasExists(context.Background(), "linux.com", "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected exists=false for 404")
	}
}

func TestCreateAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r)
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"newuser","recipients":["new@example.com"],"labels":["lfid:auth0|123"]}`))
	}))
	defer srv.Close()

	client := newTestClient(srv)
	alias, err := client.CreateAlias(context.Background(), "linux.com", &model.CreateAliasRequest{
		Name:       "newuser",
		Recipients: []string{"new@example.com"},
		Labels:     []string{"lfid:auth0|123"},
		IsEnabled:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias.Name != "newuser" {
		t.Errorf("alias.Name = %q, want %q", alias.Name, "newuser")
	}
}

func TestUpdateAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r)
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"existing","recipients":["updated@example.com"]}`))
	}))
	defer srv.Close()

	client := newTestClient(srv)
	alias, err := client.UpdateAlias(context.Background(), "linux.com", "existing", &model.UpdateAliasRequest{
		Recipients: []string{"updated@example.com"},
		IsEnabled:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alias.Recipients) == 0 || alias.Recipients[0] != "updated@example.com" {
		t.Errorf("unexpected recipients: %v", alias.Recipients)
	}
}
