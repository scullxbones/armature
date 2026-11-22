package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/clock"
)

type Lifecycle struct {
	manifestPath  string
	provider      providerRegistry
	fileCommitter worktreeCommitter
	worktreePath  string
	now           clock.Clock
}

type providerRegistry interface {
	ProviderForType(providerType string) (provider, error)
}

type defaultProviderRegistry struct{}

func (r *defaultProviderRegistry) ProviderForType(providerType string) (provider, error) {
	switch providerType {
	case "filesystem":
		return &filesystemProvider{}, nil
	case "confluence":
		return nil, fmt.Errorf("provider %q not configured: base URL and credentials are required", providerType)
	case "sharepoint":
		return nil, fmt.Errorf("provider %q not configured: base URL and credentials are required", providerType)
	default:
		return nil, fmt.Errorf("unknown provider type %q", providerType)
	}
}

// NewLifecycle creates a new source lifecycle manager.
// manifestPath is the directory where manifest.json and cache files are stored.
func NewLifecycle(manifestPath string) *Lifecycle {
	return newLifecycle(manifestPath, &defaultProviderRegistry{}, "", nil)
}

// Pass worktreePath="" to disable auto-commit.
func NewLifecycleWithCommitter(manifestPath, worktreePath string, fc worktreeCommitter) *Lifecycle {
	return newLifecycle(manifestPath, &defaultProviderRegistry{}, worktreePath, fc)
}

func newLifecycle(manifestPath string, registry providerRegistry, worktreePath string, fc worktreeCommitter) *Lifecycle {
	return &Lifecycle{
		manifestPath:  manifestPath,
		provider:      registry,
		fileCommitter: fc,
		worktreePath:  worktreePath,
		now:           clock.System,
	}
}

func (l *Lifecycle) Register(entry SourceEntry) (SourceEntry, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return SourceEntry{}, fmt.Errorf("read manifest: %w", err)
	}

	manifest.Upsert(entry)

	if err := l.writeManifest(manifest); err != nil {
		return SourceEntry{}, fmt.Errorf("write manifest: %w", err)
	}

	return entry, nil
}

type syncResult struct {
	ID           string
	Fingerprint  string
	ProviderType string
	LastSynced   time.Time
	Error        error
}

func (l *Lifecycle) syncEntry(ctx context.Context, manifest *Manifest, id string) syncResult {
	entry, ok := manifest.Get(id)
	if !ok {
		return syncResult{
			ID:    id,
			Error: fmt.Errorf("source %q not found in manifest", id),
		}
	}
	providerType := entry.ProviderType

	provider, err := l.provider.ProviderForType(entry.ProviderType)
	if err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return syncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        err,
		}
	}

	data, err := provider.Fetch(ctx, *entry)
	if err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return syncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        fmt.Errorf("fetch: %w", err),
		}
	}

	fp := Fingerprint(data)

	if err := l.writeCache(id, data); err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return syncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        fmt.Errorf("write cache: %w", err),
		}
	}

	entry.Fingerprint = fp
	entry.LastSynced = l.syncedAt()
	entry.SyncFailed = false
	manifest.Upsert(*entry)

	return syncResult{
		ID:           id,
		Fingerprint:  fp,
		ProviderType: providerType,
		LastSynced:   entry.LastSynced,
	}
}

func (l *Lifecycle) syncedAt() time.Time {
	now := l.now
	if now == nil {
		now = clock.System
	}
	return time.Unix(now(), 0).UTC()
}

// SyncAll synchronizes all sources in the manifest.
// Returns a slice of results (one per source) and a combined error if all sources failed.
func (l *Lifecycle) SyncAll(ctx context.Context) ([]syncResult, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var results []syncResult
	successCount := 0

	for id := range manifest.Entries {
		result := l.syncEntry(ctx, &manifest, id)
		results = append(results, result)
		if result.Error == nil {
			successCount++
		}
	}

	if writeErr := l.writeManifest(manifest); writeErr != nil {
		return results, fmt.Errorf("write manifest: %w", writeErr)
	}

	if successCount == 0 && len(manifest.Entries) > 0 {
		details := make([]string, 0, len(results))
		for _, r := range results {
			details = append(details, fmt.Sprintf("%s: %v", r.ID, r.Error))
		}
		return results, fmt.Errorf("all sources failed to sync: %s", strings.Join(details, "; "))
	}

	return results, nil
}

type verifyResult struct {
	ID      string
	Status  verifyStatus
	Stored  string
	Current string
	Error   error
}

// Callers match the exported Verify* constants.
type verifyStatus string

const (
	VerifyOK      verifyStatus = "OK"
	VerifyChanged verifyStatus = "CHANGED"
	VerifyMissing verifyStatus = "MISSING"
	VerifyStale   verifyStatus = "STALE"
	VerifyError   verifyStatus = "ERROR"
)

func (l *Lifecycle) verifyEntry(manifest *Manifest, id string) verifyResult {
	entry, ok := manifest.Get(id)
	if !ok {
		return verifyResult{
			ID:     id,
			Status: VerifyError,
			Error:  fmt.Errorf("source %q not found in manifest", id),
		}
	}

	if entry.SyncFailed {
		return verifyResult{
			ID:     id,
			Status: VerifyStale,
			Stored: entry.Fingerprint,
		}
	}

	data, err := ReadCache(l.manifestPath, id)
	if err != nil {
		return verifyResult{
			ID:     id,
			Status: VerifyError,
			Stored: entry.Fingerprint,
			Error:  err,
		}
	}

	if data == nil {
		return verifyResult{
			ID:     id,
			Status: VerifyMissing,
			Stored: entry.Fingerprint,
		}
	}

	currentFP := Fingerprint(data)
	if currentFP == entry.Fingerprint {
		return verifyResult{
			ID:      id,
			Status:  VerifyOK,
			Stored:  entry.Fingerprint,
			Current: currentFP,
		}
	}

	return verifyResult{
		ID:      id,
		Status:  VerifyChanged,
		Stored:  entry.Fingerprint,
		Current: currentFP,
	}
}

// VerifyAll checks freshness of all sources in the manifest.
// Returns a slice of results and an error if any source is not OK.
func (l *Lifecycle) VerifyAll() ([]verifyResult, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var results []verifyResult
	allOK := true

	for id := range manifest.Entries {
		result := l.verifyEntry(&manifest, id)
		results = append(results, result)
		if result.Status != VerifyOK {
			allOK = false
		}
	}

	if !allOK {
		return results, fmt.Errorf("one or more sources have changed or are missing")
	}

	return results, nil
}

func (l *Lifecycle) ListAll() ([]SourceEntry, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	entries := make([]SourceEntry, 0, len(manifest.Entries))
	for _, e := range manifest.Entries {
		entries = append(entries, e)
	}

	return entries, nil
}

// Content returns the cached content for a source, verifying the source is
// registered before reading its cache file. Returns nil if no cache exists.
func (l *Lifecycle) Content(id string) ([]byte, error) {
	if _, err := l.Get(id); err != nil {
		return nil, err
	}
	return ReadCache(l.manifestPath, id)
}

func (l *Lifecycle) Get(id string) (*SourceEntry, error) {
	return l.entryFromManifest(func(m Manifest) (*SourceEntry, bool) {
		return m.Get(id)
	}, fmt.Sprintf("source %q not found", id))
}

func (l *Lifecycle) GetByURL(url string) (*SourceEntry, error) {
	return l.entryFromManifest(func(m Manifest) (*SourceEntry, bool) {
		return m.GetByURL(url)
	}, fmt.Sprintf("source with URL %q not found", url))
}

func (l *Lifecycle) entryFromManifest(find func(Manifest) (*SourceEntry, bool), notFound string) (*SourceEntry, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	entry, ok := find(manifest)
	if !ok {
		return nil, errors.New(notFound)
	}
	return entry, nil
}

func (l *Lifecycle) writeManifest(manifest Manifest) error {
	if l.fileCommitter != nil && l.worktreePath != "" {
		return writeManifestAndCommit(l.manifestPath, l.worktreePath, manifest, l.fileCommitter)
	}
	return WriteManifest(l.manifestPath, manifest)
}

func (l *Lifecycle) writeCache(id string, data []byte) error {
	if l.fileCommitter != nil && l.worktreePath != "" {
		return writeCacheAndCommit(l.manifestPath, l.worktreePath, id, data, l.fileCommitter)
	}
	return WriteCache(l.manifestPath, id, data)
}
