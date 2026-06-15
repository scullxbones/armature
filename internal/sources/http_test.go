package sources

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

type fakeHTTPClient struct {
	do func(*http.Request) (*http.Response, error)
}

func (c fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.do(req)
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFetchHTTP_BearerToken(t *testing.T) {
	t.Parallel()
	const wantBody = `{"data":"hello"}`
	const token = "my-bearer-token"

	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer "+token {
			return testResponse(http.StatusUnauthorized, "unauthorized"), nil
		}
		return testResponse(http.StatusOK, wantBody), nil
	}}

	got, err := adapters.FetchHTTP(context.Background(), client, "https://example.test/", "", "", token)
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	t.Parallel()
	const wantBody = `{"result":"ok"}`

	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			return testResponse(http.StatusUnauthorized, "unauthorized"), nil
		}
		return testResponse(http.StatusOK, wantBody), nil
	}}

	got, err := adapters.FetchHTTP(context.Background(), client, "https://example.test/", "alice", "secret", "")
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_ErrorStatus(t *testing.T) {
	t.Parallel()
	client := fakeHTTPClient{do: func(_ *http.Request) (*http.Response, error) {
		return testResponse(http.StatusNotFound, "not found"), nil
	}}

	_, err := adapters.FetchHTTP(context.Background(), client, "https://example.test/missing", "", "", "")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestFetchHTTP_NoAuth(t *testing.T) {
	t.Parallel()
	const wantBody = `{"public":"data"}`

	client := fakeHTTPClient{do: func(_ *http.Request) (*http.Response, error) {
		return testResponse(http.StatusOK, wantBody), nil
	}}

	got, err := adapters.FetchHTTP(context.Background(), client, "https://example.test/", "", "", "")
	if err != nil {
		t.Fatalf("FetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}
