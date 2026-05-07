package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient(t *testing.T) {
	c := NewHTTPClient()
	if c == nil {
		t.Fatal("expected non-nil http client")
	}
}

func TestFetchHTTP_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	body, err := FetchHTTP(context.Background(), srv.Client(), srv.URL, "", "", "mytoken")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`data`))
	}))
	defer srv.Close()

	body, err := FetchHTTP(context.Background(), srv.Client(), srv.URL, "user", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "data" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestFetchHTTP_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchHTTP(context.Background(), srv.Client(), srv.URL, "", "", "")
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
