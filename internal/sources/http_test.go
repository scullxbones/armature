package sources

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

// mockTransport is shared across test files in this package.
type mockTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return m.fn(r)
}

func mockClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: &mockTransport{fn: fn}}
}

func mockResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestFetchHTTP_BearerToken(t *testing.T) {
	const wantBody = `{"data":"hello"}`
	const token = "my-bearer-token"

	client := mockClient(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			return mockResp(http.StatusUnauthorized, "unauthorized"), nil
		}
		return mockResp(http.StatusOK, wantBody), nil
	})

	got, err := adapters.FetchHTTP(context.Background(), client, "http://example.com/", "", "", token)
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	const wantBody = `{"result":"ok"}`

	client := mockClient(func(r *http.Request) (*http.Response, error) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			return mockResp(http.StatusUnauthorized, "unauthorized"), nil
		}
		return mockResp(http.StatusOK, wantBody), nil
	})

	got, err := adapters.FetchHTTP(context.Background(), client, "http://example.com/", "alice", "secret", "")
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_ErrorStatus(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return mockResp(http.StatusNotFound, "not found"), nil
	})

	_, err := adapters.FetchHTTP(context.Background(), client, "http://example.com/missing", "", "", "")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestFetchHTTP_NoAuth(t *testing.T) {
	const wantBody = `{"public":"data"}`

	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return mockResp(http.StatusOK, wantBody), nil
	})

	got, err := adapters.FetchHTTP(context.Background(), client, "http://example.com/", "", "", "")
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}
