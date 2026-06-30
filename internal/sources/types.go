package sources

import (
	"context"
	"encoding/json"
	"time"
)

// SourceEntry represents a single tracked source document.
type SourceEntry struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Fingerprint  string    `json:"fingerprint"`
	LastSynced   time.Time `json:"last_synced"`
	ProviderType string    `json:"provider_type"`
}

// Manifest holds a collection of SourceEntries keyed by ID.
type Manifest struct {
	Entries map[string]SourceEntry `json:"entries"`
}

// Get returns the SourceEntry with the given ID and a boolean indicating
// whether it was found.
func (m *Manifest) Get(id string) (*SourceEntry, bool) {
	if m.Entries == nil {
		return nil, false
	}
	e, ok := m.Entries[id]
	if !ok {
		return nil, false
	}
	return &e, true
}

// GetByURL returns the first SourceEntry whose URL matches the given value,
// along with a boolean indicating whether it was found.
func (m *Manifest) GetByURL(url string) (*SourceEntry, bool) {
	if m.Entries == nil {
		return nil, false
	}
	for _, e := range m.Entries {
		if e.URL == url {
			entry := e
			return &entry, true
		}
	}
	return nil, false
}

// Upsert inserts or replaces the SourceEntry in the manifest.
func (m *Manifest) Upsert(entry SourceEntry) {
	if m.Entries == nil {
		m.Entries = make(map[string]SourceEntry)
	}
	m.Entries[entry.ID] = entry
}

// Marshal encodes the manifest to JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal decodes JSON data into the manifest.
func (m *Manifest) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// Provider is implemented by any source that can fetch document content.
type Provider interface {
	// Fetch retrieves the raw content for the given source entry.
	Fetch(ctx context.Context, entry SourceEntry) ([]byte, error)
	// Type returns the provider type identifier (e.g. "github", "confluence").
	Type() string
}
