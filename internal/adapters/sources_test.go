package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

func TestNewHTTPClient(t *testing.T) {
	c := NewHTTPClient()
	if c == nil {
		t.Fatal("expected non-nil http client")
	}
}

func TestFetchHTTP_BearerAuth(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			return mockResp(http.StatusUnauthorized, ""), nil
		}
		return mockResp(http.StatusOK, `{"ok":true}`), nil
	})

	body, err := FetchHTTP(context.Background(), client, "http://example.com", "", "", "mytoken")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			return mockResp(http.StatusUnauthorized, ""), nil
		}
		return mockResp(http.StatusOK, "data"), nil
	})

	body, err := FetchHTTP(context.Background(), client, "http://example.com", "user", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "data" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_Non2xx(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return mockResp(http.StatusNotFound, ""), nil
	})

	_, err := FetchHTTP(context.Background(), client, "http://example.com", "", "", "")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetchHTTP_InvalidURL(t *testing.T) {
	_, err := FetchHTTP(context.Background(), &http.Client{}, "://bad-url", "", "", "")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
