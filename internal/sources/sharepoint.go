package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// SharePointProvider implements Provider for Microsoft SharePoint sources.
type SharePointProvider struct {
	baseURL string
	creds   Credentials
	client  *http.Client
}

// NewSharePointProvider returns a new SharePointProvider targeting baseURL
// and authenticating with the supplied Credentials.
func NewSharePointProvider(baseURL string, creds Credentials) *SharePointProvider {
	return &SharePointProvider{
		baseURL: baseURL,
		creds:   creds,
		client:  &http.Client{},
	}
}

// Type returns the provider type identifier for SharePoint sources.
func (p *SharePointProvider) Type() string {
	return "sharepoint"
}

// Fetch retrieves the raw content for the given source entry from SharePoint.
// It makes an HTTP GET request to baseURL + entry.URL using Bearer
// authentication from the configured Credentials Token.
func (p *SharePointProvider) Fetch(ctx context.Context, entry SourceEntry) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+entry.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("sharepoint provider: create request: %w", err)
	}

	if p.creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.creds.Token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sharepoint provider: fetch %q: %w", entry.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sharepoint provider: fetch %q: unexpected status %d", entry.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sharepoint provider: read body %q: %w", entry.URL, err)
	}

	return body, nil
}
