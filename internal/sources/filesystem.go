package sources

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
)

// FilesystemProvider implements Provider for local filesystem paths.
//
// Fetch reads the file at entry.URL (treated as a local path) and returns its
// raw content. No remote version ID exists for local files. Callers that need
// to persist the fingerprint should call Fingerprint on the returned bytes and
// store it in entry.Fingerprint before upserting into the Manifest.
type FilesystemProvider struct{}

func (p *FilesystemProvider) Type() string {
	return "filesystem"
}

func (p *FilesystemProvider) Fetch(_ context.Context, entry SourceEntry) ([]byte, error) {
	data, err := adapters.ReadFile(entry.URL)
	if err != nil {
		return nil, fmt.Errorf("filesystem provider: read %q: %w", entry.URL, err)
	}
	return data, nil
}
