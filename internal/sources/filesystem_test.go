package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemProviderType(t *testing.T) {
	t.Parallel()
	p := &FilesystemProvider{}
	if got := p.Type(); got != "filesystem" {
		t.Errorf("Type() = %q; want %q", got, "filesystem")
	}
}

func TestFilesystemFetchContent(t *testing.T) {
	t.Parallel()
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

func TestFilesystemFetchFingerprint(t *testing.T) {
	t.Parallel()
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

func TestFilesystemFetchEmptyVersionID(t *testing.T) {
	t.Parallel()
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

	entry.Fingerprint = Fingerprint(data)
	if entry.Fingerprint == "" {
		t.Error("expected non-empty fingerprint after computing from fetched content")
	}
}

func TestFilesystemFetchMissingFile(t *testing.T) {
	t.Parallel()
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

func TestFilesystemImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = &FilesystemProvider{}
}
