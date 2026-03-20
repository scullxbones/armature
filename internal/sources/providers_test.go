package sources_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scullxbones/trellis/internal/sources"
)

func TestConfluenceProviderFetch(t *testing.T) {
	const expectedBody = `{"title":"Test Page","body":"hello confluence"}`
	const token = "test-confluence-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wiki/pages/42" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	}))
	defer srv.Close()

	creds := sources.Credentials{Token: token}
	provider := sources.NewConfluenceProvider(srv.URL, creds)

	if provider.Type() != "confluence" {
		t.Fatalf("expected Type() == %q, got %q", "confluence", provider.Type())
	}

	entry := sources.SourceEntry{
		ID:  "page-42",
		URL: "/wiki/pages/42",
	}

	got, err := provider.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}

	if string(got) != expectedBody {
		t.Errorf("Fetch body mismatch:\n  got:  %q\n  want: %q", string(got), expectedBody)
	}
}

func TestConfluenceProviderFetchBasicAuth(t *testing.T) {
	const expectedBody = `{"result":"ok"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	}))
	defer srv.Close()

	creds := sources.Credentials{Username: "admin", Password: "secret"}
	provider := sources.NewConfluenceProvider(srv.URL, creds)

	entry := sources.SourceEntry{ID: "doc-1", URL: "/"}

	got, err := provider.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}

	if string(got) != expectedBody {
		t.Errorf("Fetch body mismatch:\n  got:  %q\n  want: %q", string(got), expectedBody)
	}
}

func TestSharePointProviderFetch(t *testing.T) {
	const expectedBody = `{"value":"SharePoint document content"}`
	const token = "test-sharepoint-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites/docs/item/99" {
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	}))
	defer srv.Close()

	creds := sources.Credentials{Token: token}
	provider := sources.NewSharePointProvider(srv.URL, creds)

	if provider.Type() != "sharepoint" {
		t.Fatalf("expected Type() == %q, got %q", "sharepoint", provider.Type())
	}

	entry := sources.SourceEntry{
		ID:  "item-99",
		URL: "/sites/docs/item/99",
	}

	got, err := provider.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}

	if string(got) != expectedBody {
		t.Errorf("Fetch body mismatch:\n  got:  %q\n  want: %q", string(got), expectedBody)
	}
}

func TestSharePointProviderFetchErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	provider := sources.NewSharePointProvider(srv.URL, sources.Credentials{Token: "tok"})
	entry := sources.SourceEntry{ID: "x", URL: "/missing"}

	_, err := provider.Fetch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}
