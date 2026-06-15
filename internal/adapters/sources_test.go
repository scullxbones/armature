package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
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

func TestNewHTTPClient(t *testing.T) {
	t.Parallel()
	c := NewHTTPClient()
	if c == nil {
		t.Fatal("expected non-nil http client")
	}
}

func TestFetchHTTP_BearerAuth(t *testing.T) {
	t.Parallel()
	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer mytoken" {
			return testResponse(http.StatusUnauthorized, ""), nil
		}
		return testResponse(http.StatusOK, `{"ok":true}`), nil
	}}

	body, err := FetchHTTP(context.Background(), client, "https://example.test/resource", "", "", "mytoken")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	t.Parallel()
	client := fakeHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		u, p, ok := req.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			return testResponse(http.StatusUnauthorized, ""), nil
		}
		return testResponse(http.StatusOK, "data"), nil
	}}

	body, err := FetchHTTP(context.Background(), client, "https://example.test/resource", "user", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "data" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_Non2xx(t *testing.T) {
	t.Parallel()
	client := fakeHTTPClient{do: func(_ *http.Request) (*http.Response, error) {
		return testResponse(http.StatusNotFound, ""), nil
	}}

	_, err := FetchHTTP(context.Background(), client, "https://example.test/missing", "", "", "")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetchHTTP_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := FetchHTTP(context.Background(), &http.Client{}, "://bad-url", "", "", "")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
