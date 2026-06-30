package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchHTTP_BearerToken(t *testing.T) {
	const wantBody = `{"data":"hello"}`
	const token = "my-bearer-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	}))
	defer srv.Close()

	creds := Credentials{Token: token}
	client := &http.Client{}

	got, err := fetchHTTP(context.Background(), client, srv.URL+"/", creds)
	if err != nil {
		t.Fatalf("fetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_BasicAuth(t *testing.T) {
	const wantBody = `{"result":"ok"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	}))
	defer srv.Close()

	creds := Credentials{Username: "alice", Password: "secret"}
	client := &http.Client{}

	got, err := fetchHTTP(context.Background(), client, srv.URL+"/", creds)
	if err != nil {
		t.Fatalf("fetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}

func TestFetchHTTP_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := &http.Client{}

	_, err := fetchHTTP(context.Background(), client, srv.URL+"/missing", Credentials{})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestFetchHTTP_NoAuth(t *testing.T) {
	const wantBody = `{"public":"data"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	}))
	defer srv.Close()

	client := &http.Client{}

	got, err := fetchHTTP(context.Background(), client, srv.URL+"/", Credentials{})
	if err != nil {
		t.Fatalf("fetchHTTP returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body mismatch: got %q, want %q", string(got), wantBody)
	}
}
