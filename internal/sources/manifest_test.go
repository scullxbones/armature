package sources

import (
	"testing"
	"time"
)

func TestManifestPersistence(t *testing.T) {
	dir := t.TempDir()

	entry := SourceEntry{
		ID:           "doc-1",
		URL:          "https://example.com/doc-1",
		Title:        "Test Document",
		Fingerprint:  "abc123",
		LastSynced:   time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		ProviderType: "github",
	}

	m := Manifest{}
	m.Upsert(entry)

	if err := WriteManifest(dir, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	gotEntry, ok := got.Get("doc-1")
	if !ok {
		t.Fatal("expected entry 'doc-1' not found in read manifest")
	}

	if gotEntry.ID != entry.ID {
		t.Errorf("ID: got %q, want %q", gotEntry.ID, entry.ID)
	}
	if gotEntry.URL != entry.URL {
		t.Errorf("URL: got %q, want %q", gotEntry.URL, entry.URL)
	}
	if gotEntry.Title != entry.Title {
		t.Errorf("Title: got %q, want %q", gotEntry.Title, entry.Title)
	}
	if gotEntry.Fingerprint != entry.Fingerprint {
		t.Errorf("Fingerprint: got %q, want %q", gotEntry.Fingerprint, entry.Fingerprint)
	}
	if !gotEntry.LastSynced.Equal(entry.LastSynced) {
		t.Errorf("LastSynced: got %v, want %v", gotEntry.LastSynced, entry.LastSynced)
	}
	if gotEntry.ProviderType != entry.ProviderType {
		t.Errorf("ProviderType: got %q, want %q", gotEntry.ProviderType, entry.ProviderType)
	}
}

func TestReadManifestMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	// Use a subdirectory that definitely doesn't have a manifest.json
	nonexistent := dir + "/no-such-dir"

	m, err := ReadManifest(nonexistent)
	if err != nil {
		t.Fatalf("expected no error for missing manifest, got: %v", err)
	}
	if len(m.Entries) != 0 {
		t.Errorf("expected empty manifest, got %d entries", len(m.Entries))
	}
}
