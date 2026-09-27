// Package sources ingests external reference material (filesystem, Confluence, SharePoint)
// used to back citations, tracking manifest and freshness fingerprints.
package sources

import (
	"context"
	"encoding/json"
	"time"
)

type SourceEntry struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Fingerprint  string    `json:"fingerprint"`
	LastSynced   time.Time `json:"last_synced"`
	ProviderType string    `json:"provider_type"`
	SyncFailed   bool      `json:"sync_failed"`
}

type Manifest struct {
	Entries map[string]SourceEntry `json:"entries"`
}

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

func (m *Manifest) Upsert(entry SourceEntry) {
	if m.Entries == nil {
		m.Entries = make(map[string]SourceEntry)
	}
	m.Entries[entry.ID] = entry
}

func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func (m *Manifest) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

type Provider interface {
	Fetch(ctx context.Context, entry SourceEntry) ([]byte, error)
	Type() string
}
