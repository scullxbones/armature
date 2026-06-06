package sources

import (
	"context"
	"net/http"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

func newConfluenceWithMock(fn func(*http.Request) (*http.Response, error), baseURL string, creds Credentials) *ConfluenceProvider {
	return &ConfluenceProvider{baseURL: baseURL, creds: creds, client: mockClient(fn)}
}

func newSharePointWithMock(fn func(*http.Request) (*http.Response, error), baseURL string, creds Credentials) *SharePointProvider {
	return &SharePointProvider{baseURL: baseURL, creds: creds, client: mockClient(fn)}
}

func TestConfluenceProviderFetch(t *testing.T) {
	const expectedBody = `{"title":"Test Page","body":"hello confluence"}`
	const token = "test-confluence-token"

	provider := newConfluenceWithMock(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/wiki/pages/42" {
			return mockResp(http.StatusNotFound, "not found"), nil
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			return mockResp(http.StatusUnauthorized, "unauthorized"), nil
		}
		resp := mockResp(http.StatusOK, expectedBody)
		resp.Header.Set("Content-Type", "application/json")
		return resp, nil
	}, "https://example.test", Credentials{Token: token})

	if provider.Type() != "confluence" {
		t.Fatalf("expected Type() == %q, got %q", "confluence", provider.Type())
	}

	entry := SourceEntry{ID: "page-42", URL: "/wiki/pages/42"}

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

	provider := newConfluenceWithMock(func(r *http.Request) (*http.Response, error) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "admin" || p != "secret" {
			return mockResp(http.StatusUnauthorized, "unauthorized"), nil
		}
		return mockResp(http.StatusOK, expectedBody), nil
	}, "https://example.test", Credentials{Username: "admin", Password: "secret"})

	entry := SourceEntry{ID: "doc-1", URL: "/"}

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

	provider := newSharePointWithMock(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/sites/docs/item/99" {
			return mockResp(http.StatusNotFound, "not found"), nil
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			return mockResp(http.StatusUnauthorized, "unauthorized"), nil
		}
		resp := mockResp(http.StatusOK, expectedBody)
		resp.Header.Set("Content-Type", "application/json")
		return resp, nil
	}, "https://example.test", Credentials{Token: token})

	if provider.Type() != "sharepoint" {
		t.Fatalf("expected Type() == %q, got %q", "sharepoint", provider.Type())
	}

	entry := SourceEntry{ID: "item-99", URL: "/sites/docs/item/99"}

	got, err := provider.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if string(got) != expectedBody {
		t.Errorf("Fetch body mismatch:\n  got:  %q\n  want: %q", string(got), expectedBody)
	}
}

func TestSharePointProviderFetchErrorStatus(t *testing.T) {
	provider := newSharePointWithMock(func(r *http.Request) (*http.Response, error) {
		return mockResp(http.StatusNotFound, "not found"), nil
	}, "https://example.test", Credentials{Token: "tok"})

	entry := SourceEntry{ID: "x", URL: "/missing"}

	_, err := provider.Fetch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// Ensure the test helpers satisfy the adapters.HTTPClient interface.
var _ adapters.HTTPClient = (*http.Client)(nil)
