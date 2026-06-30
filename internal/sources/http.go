package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// fetchHTTP performs an authenticated HTTP GET to url using the given client
// and credentials. If a Token is set, Bearer auth is used; otherwise Basic
// auth is applied when Username or Password is non-empty.
// Returns the response body or an error for non-2xx status codes.
func fetchHTTP(ctx context.Context, client *http.Client, url string, creds Credentials) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+creds.Token)
	} else if creds.Username != "" || creds.Password != "" {
		req.SetBasicAuth(creds.Username, creds.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %q: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %q: %w", url, err)
	}

	return body, nil
}
