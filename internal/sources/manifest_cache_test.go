package sources_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/sources"
)

func TestWriteAndReadCache(t *testing.T) {
	dir := t.TempDir()
	data := []byte("cached content")

	if err := sources.WriteCache(dir, "entry-1", data); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	got, err := sources.ReadCache(dir, "entry-1")
	if err != nil {
		t.Fatalf("ReadCache: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("ReadCache: got %q, want %q", string(got), string(data))
	}
}

func TestReadCache_Missing_ReturnsNil(t *testing.T) {
	dir := t.TempDir()

	got, err := sources.ReadCache(dir, "nonexistent")
	if err != nil {
		t.Fatalf("ReadCache of missing entry returned error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing cache, got %q", string(got))
	}
}

func TestManifestGet_NilEntries_ReturnsFalse(t *testing.T) {
	var m sources.Manifest
	_, ok := m.Get("any")
	if ok {
		t.Error("expected Get on nil-entries Manifest to return false")
	}
}

func TestManifestGet_NotFound_ReturnsFalse(t *testing.T) {
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{ID: "a", URL: "/a"})

	_, ok := m.Get("b")
	if ok {
		t.Error("expected Get of missing ID to return false")
	}
}

func TestManifestGet_Found_ReturnsEntry(t *testing.T) {
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{ID: "a", URL: "/a", Title: "Alpha"})

	got, ok := m.Get("a")
	if !ok {
		t.Fatal("expected Get to return true for existing entry")
	}
	if got.Title != "Alpha" {
		t.Errorf("expected Title %q, got %q", "Alpha", got.Title)
	}
}

func TestWriteManifest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{ID: "doc-1", URL: "/docs/1", Title: "Doc One"})

	if err := sources.WriteManifest(dir, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := sources.ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	entry, ok := got.Get("doc-1")
	if !ok {
		t.Fatal("expected entry doc-1 in read manifest")
	}
	if entry.Title != "Doc One" {
		t.Errorf("expected Title %q, got %q", "Doc One", entry.Title)
	}
}
