package sources

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
)

// SharePointProvider implements Provider for Microsoft SharePoint sources.
type SharePointProvider struct {
	baseURL string
	creds   Credentials
	client  adapters.HTTPClient
}

// NewSharePointProvider returns a new SharePointProvider targeting baseURL
// and authenticating with the supplied Credentials.
func NewSharePointProvider(baseURL string, creds Credentials) *SharePointProvider {
	return &SharePointProvider{
		baseURL: baseURL,
		creds:   creds,
		client:  adapters.NewHTTPClient(),
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
	body, err := adapters.FetchHTTP(ctx, p.client, p.baseURL+entry.URL, p.creds.Username, p.creds.Password, p.creds.Token)
	if err != nil {
		return nil, fmt.Errorf("sharepoint provider: %w", err)
	}
	return body, nil
}
