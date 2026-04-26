package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestFilesystemProviderType verifies that FilesystemProvider reports the
// correct type identifier.
func TestFilesystemProviderType(t *testing.T) {
	p := &FilesystemProvider{}
	if got := p.Type(); got != "filesystem" {
		t.Errorf("Type() = %q; want %q", got, "filesystem")
	}
}

// TestFilesystemFetchContent writes a temp file and asserts that Fetch returns
// the expected content.
func TestFilesystemFetchContent(t *testing.T) {
	content := []byte("hello armature filesystem provider")

	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	p := &FilesystemProvider{}
	entry := SourceEntry{
		ID:           "test-id",
		URL:          path,
		ProviderType: "filesystem",
	}

	got, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch() error = %v; want nil", err)
	}

	if string(got) != string(content) {
		t.Errorf("Fetch() content = %q; want %q", got, content)
	}
}

// TestFilesystemFetchFingerprint verifies that the SHA-256 fingerprint of the
// fetched content matches the expected digest. The Provider interface returns
// []byte; callers compute the fingerprint via Fingerprint(content).
func TestFilesystemFetchFingerprint(t *testing.T) {
	content := []byte("hello armature filesystem provider")
	expected := Fingerprint(content)

	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	p := &FilesystemProvider{}
	entry := SourceEntry{
		ID:           "test-fp",
		URL:          path,
		ProviderType: "filesystem",
	}

	got, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch() error = %v; want nil", err)
	}

	gotFP := Fingerprint(got)
	if gotFP != expected {
		t.Errorf("Fingerprint(Fetch()) = %q; want %q", gotFP, expected)
	}
}

// TestFilesystemFetchEmptyVersionID documents that local filesystem sources
// carry no remote version ID. This is verified by confirming Fetch succeeds
// and the caller is responsible for setting entry.Fingerprint from the content.
func TestFilesystemFetchEmptyVersionID(t *testing.T) {
	content := []byte("version id test content")

	dir := t.TempDir()
	path := filepath.Join(dir, "version_test.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	p := &FilesystemProvider{}
	entry := SourceEntry{
		ID:           "test-version",
		URL:          path,
		ProviderType: "filesystem",
	}

	data, err := p.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch() error = %v; want nil", err)
	}

	// Demonstrate the caller pattern: compute fingerprint and set on entry.
	entry.Fingerprint = Fingerprint(data)
	// VersionID is not part of SourceEntry — it is empty/unused for filesystem.
	if entry.Fingerprint == "" {
		t.Error("expected non-empty fingerprint after computing from fetched content")
	}
}

// TestFilesystemFetchMissingFile verifies that Fetch returns an error when the
// file does not exist.
func TestFilesystemFetchMissingFile(t *testing.T) {
	p := &FilesystemProvider{}
	entry := SourceEntry{
		ID:           "missing",
		URL:          "/nonexistent/path/does_not_exist.txt",
		ProviderType: "filesystem",
	}

	_, err := p.Fetch(context.Background(), entry)
	if err == nil {
		t.Error("Fetch() error = nil; want non-nil for missing file")
	}
}

// TestFilesystemImplementsProvider ensures FilesystemProvider satisfies the
// Provider interface at compile time.
func TestFilesystemImplementsProvider(t *testing.T) {
	var _ Provider = &FilesystemProvider{}
}
