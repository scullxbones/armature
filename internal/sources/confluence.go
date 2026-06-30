package sources

import (
	"context"
	"fmt"
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
	body, err := fetchHTTP(ctx, p.client, p.baseURL+entry.URL, p.creds)
	if err != nil {
		return nil, fmt.Errorf("confluence provider: %w", err)
	}
	return body, nil
}
