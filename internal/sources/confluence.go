package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// ConfluenceProvider implements Provider for Atlassian Confluence sources.
type ConfluenceProvider struct {
	baseURL string
	creds   Credentials
	client  *http.Client
}

// NewConfluenceProvider returns a new ConfluenceProvider targeting baseURL
// and authenticating with the supplied Credentials.
func NewConfluenceProvider(baseURL string, creds Credentials) *ConfluenceProvider {
	return &ConfluenceProvider{
		baseURL: baseURL,
		creds:   creds,
		client:  &http.Client{},
	}
}

// Type returns the provider type identifier for Confluence sources.
func (p *ConfluenceProvider) Type() string {
	return "confluence"
}

// Fetch retrieves the raw content for the given source entry from Confluence.
// It makes an HTTP GET request to baseURL + entry.URL. If a Token is set it
// uses Bearer authentication; otherwise it falls back to Basic auth using
// Username and Password.
func (p *ConfluenceProvider) Fetch(ctx context.Context, entry SourceEntry) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+entry.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("confluence provider: create request: %w", err)
	}

	if p.creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.creds.Token)
	} else if p.creds.Username != "" || p.creds.Password != "" {
		req.SetBasicAuth(p.creds.Username, p.creds.Password)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("confluence provider: fetch %q: %w", entry.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("confluence provider: fetch %q: unexpected status %d", entry.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("confluence provider: read body %q: %w", entry.URL, err)
	}

	return body, nil
}
