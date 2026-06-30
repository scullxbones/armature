package adapters

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// ===== HTTP Client Interface (for sources providers) =====

// HTTPClient is an interface for HTTP clients used by source providers.
// This allows sources to use adapters-provided clients without importing net/http.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ===== HTTP Provider Logic (from sources/http.go, sources/confluence.go, sources/sharepoint.go) =====

// FetchHTTP performs an authenticated HTTP GET to url using the given client
// and credentials. If a Token is set, Bearer auth is used; otherwise Basic
// auth is applied when Username or Password is non-empty.
// Returns the response body or an error for non-2xx status codes.
func FetchHTTP(ctx context.Context, client HTTPClient, url string, username, password, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // close error in defer not actionable

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %q: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %q: %w", url, err)
	}

	return body, nil
}

// NewHTTPClient creates a new HTTP client for making requests.
// Returns an HTTPClient interface so sources don't need to import net/http.
func NewHTTPClient() HTTPClient {
	return &http.Client{}
}
