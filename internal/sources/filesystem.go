package sources

import (
	"context"
	"fmt"
	"os"
)

// FilesystemProvider implements Provider for local filesystem paths.
//
// Fetch reads the file at entry.URL (treated as a local path) and returns its
// raw content. The SHA-256 fingerprint of the returned content can be obtained
// by calling Fingerprint(content). No remote version ID exists for local files,
// so versionID is effectively empty — callers that need to persist the
// fingerprint should call Fingerprint on the returned bytes and store it in
// entry.Fingerprint before upserting into the Manifest.
type FilesystemProvider struct{}

// Type returns the provider type identifier for filesystem sources.
func (p *FilesystemProvider) Type() string {
	return "filesystem"
}

// Fetch reads the file at entry.URL (local path) and returns its content.
// The SHA-256 fingerprint of the content is computed via Fingerprint(content).
// Version ID is empty for local files (no remote versioning).
func (p *FilesystemProvider) Fetch(_ context.Context, entry SourceEntry) ([]byte, error) {
	data, err := os.ReadFile(entry.URL)
	if err != nil {
		return nil, fmt.Errorf("filesystem provider: read %q: %w", entry.URL, err)
	}
	return data, nil
}
