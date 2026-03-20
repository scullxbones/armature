package sources

import (
	"testing"
	"time"
)

func TestManifestRoundTrip(t *testing.T) {
	entry := SourceEntry{
		ID:           "src-001",
		URL:          "https://example.com/doc",
		Title:        "Example Document",
		Fingerprint:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		LastSynced:   time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC),
		ProviderType: "github",
	}

	var m1 Manifest
	m1.Upsert(entry)

	data, err := m1.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m2 Manifest
	if err := m2.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	got, ok := m2.Get(entry.ID)
	if !ok {
		t.Fatalf("Get(%q): entry not found after round-trip", entry.ID)
	}
	if got.ID != entry.ID {
		t.Errorf("ID: got %q, want %q", got.ID, entry.ID)
	}
	if got.URL != entry.URL {
		t.Errorf("URL: got %q, want %q", got.URL, entry.URL)
	}
	if got.Title != entry.Title {
		t.Errorf("Title: got %q, want %q", got.Title, entry.Title)
	}
	if got.Fingerprint != entry.Fingerprint {
		t.Errorf("Fingerprint: got %q, want %q", got.Fingerprint, entry.Fingerprint)
	}
	if !got.LastSynced.Equal(entry.LastSynced) {
		t.Errorf("LastSynced: got %v, want %v", got.LastSynced, entry.LastSynced)
	}
	if got.ProviderType != entry.ProviderType {
		t.Errorf("ProviderType: got %q, want %q", got.ProviderType, entry.ProviderType)
	}
}
