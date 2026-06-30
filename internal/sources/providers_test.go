package sources

import (
	"context"
	"net/http"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

func TestConfluenceProviderFetch(t *testing.T) {
	t.Parallel()
	const expectedBody = `{"title":"Test Page","body":"hello confluence"}`
	const token = "test-confluence-token"

	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/wiki/pages/42" {
			return testResponse(http.StatusNotFound, "not found"), nil
		}
		auth := req.Header.Get("Authorization")
		if auth != "Bearer "+token {
			return testResponse(http.StatusUnauthorized, "unauthorized"), nil
		}
		return testResponse(http.StatusOK, expectedBody), nil
	}}

	creds := Credentials{Token: token}
	provider := NewConfluenceProvider("https://example.test", creds)
	provider.client = client

	if provider.Type() != "confluence" {
		t.Fatalf("expected Type() == %q, got %q", "confluence", provider.Type())
	}

	entry := SourceEntry{
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
	t.Parallel()
	const expectedBody = `{"result":"ok"}`

	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			return testResponse(http.StatusUnauthorized, "unauthorized"), nil
		}
		return testResponse(http.StatusOK, expectedBody), nil
	}}

	creds := Credentials{Username: "admin", Password: "secret"}
	provider := NewConfluenceProvider("https://example.test", creds)
	provider.client = client

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
	t.Parallel()
	const expectedBody = `{"value":"SharePoint document content"}`
	const token = "test-sharepoint-token"

	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/sites/docs/item/99" {
			return testResponse(http.StatusNotFound, "not found"), nil
		}
		auth := req.Header.Get("Authorization")
		if auth != "Bearer "+token {
			return testResponse(http.StatusUnauthorized, "unauthorized"), nil
		}
		return testResponse(http.StatusOK, expectedBody), nil
	}}

	creds := Credentials{Token: token}
	provider := NewSharePointProvider("https://example.test", creds)
	provider.client = client

	if provider.Type() != "sharepoint" {
		t.Fatalf("expected Type() == %q, got %q", "sharepoint", provider.Type())
	}

	entry := SourceEntry{
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
	t.Parallel()
	client := fakeHTTPClient{do: func(_ *http.Request) (*http.Response, error) {
		return testResponse(http.StatusNotFound, "not found"), nil
	}}

	provider := NewSharePointProvider("https://example.test", Credentials{Token: "tok"})
	provider.client = client

	entry := SourceEntry{ID: "x", URL: "/missing"}

	_, err := provider.Fetch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// Ensure the test helpers satisfy the adapters.HTTPClient interface.
var _ adapters.HTTPClient = (*http.Client)(nil)
