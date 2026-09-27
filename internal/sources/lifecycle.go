package sources

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Lifecycle struct {
	manifestPath  string
	provider      ProviderRegistry
	fileCommitter FileCommitter
	worktreePath  string
}

type ProviderRegistry interface {
	ProviderForType(providerType string) (Provider, error)
}

type DefaultProviderRegistry struct{}

func (r *DefaultProviderRegistry) ProviderForType(providerType string) (Provider, error) {
	switch providerType {
	case "filesystem":
		return &FilesystemProvider{}, nil
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
	return newLifecycleWithRegistry(manifestPath, &DefaultProviderRegistry{})
}

func newLifecycleWithRegistry(manifestPath string, registry ProviderRegistry) *Lifecycle {
	return &Lifecycle{
		manifestPath: manifestPath,
		provider:     registry,
	}
}

// NewLifecycleWithCommitter creates a new source lifecycle manager that auto-commits
// manifest and cache changes to the worktree's _armature branch.
// Pass worktreePath="" to disable auto-commit.
func NewLifecycleWithCommitter(manifestPath string, registry ProviderRegistry, worktreePath string, fc FileCommitter) *Lifecycle {
	return &Lifecycle{
		manifestPath:  manifestPath,
		provider:      registry,
		fileCommitter: fc,
		worktreePath:  worktreePath,
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

type SyncResult struct {
	ID           string
	Fingerprint  string
	ProviderType string
	LastSynced   time.Time
	Error        error
}

func (l *Lifecycle) syncEntry(ctx context.Context, manifest *Manifest, id string) SyncResult {
	entry, ok := manifest.Get(id)
	if !ok {
		return SyncResult{
			ID:    id,
			Error: fmt.Errorf("source %q not found in manifest", id),
		}
	}
	providerType := entry.ProviderType

	provider, err := l.provider.ProviderForType(entry.ProviderType)
	if err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return SyncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        err,
		}
	}

	data, err := provider.Fetch(ctx, *entry)
	if err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return SyncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        fmt.Errorf("fetch: %w", err),
		}
	}

	fp := Fingerprint(data)

	if err := l.writeCache(id, data); err != nil {
		entry.SyncFailed = true
		manifest.Upsert(*entry)
		return SyncResult{
			ID:           id,
			ProviderType: providerType,
			Error:        fmt.Errorf("write cache: %w", err),
		}
	}

	entry.Fingerprint = fp
	entry.LastSynced = time.Now().UTC() //nolint:forbidigo // sync records wall-clock time of update
	entry.SyncFailed = false
	manifest.Upsert(*entry)

	return SyncResult{
		ID:           id,
		Fingerprint:  fp,
		ProviderType: providerType,
		LastSynced:   entry.LastSynced,
	}
}

// SyncAll synchronizes all sources in the manifest.
// Returns a slice of results (one per source) and a combined error if all sources failed.
func (l *Lifecycle) SyncAll(ctx context.Context) ([]SyncResult, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var results []SyncResult
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

type VerifyResult struct {
	ID      string
	Status  VerifyStatus
	Stored  string
	Current string
	Error   error
}

type VerifyStatus string

const (
	VerifyOK      VerifyStatus = "OK"
	VerifyChanged VerifyStatus = "CHANGED"
	VerifyMissing VerifyStatus = "MISSING"
	VerifyStale   VerifyStatus = "STALE"
	VerifyError   VerifyStatus = "ERROR"
)

func (l *Lifecycle) verifyEntry(manifest *Manifest, id string) VerifyResult {
	entry, ok := manifest.Get(id)
	if !ok {
		return VerifyResult{
			ID:     id,
			Status: VerifyError,
			Error:  fmt.Errorf("source %q not found in manifest", id),
		}
	}

	if entry.SyncFailed {
		return VerifyResult{
			ID:     id,
			Status: VerifyStale,
			Stored: entry.Fingerprint,
		}
	}

	data, err := ReadCache(l.manifestPath, id)
	if err != nil {
		return VerifyResult{
			ID:     id,
			Status: VerifyError,
			Stored: entry.Fingerprint,
			Error:  err,
		}
	}

	if data == nil {
		return VerifyResult{
			ID:     id,
			Status: VerifyMissing,
			Stored: entry.Fingerprint,
		}
	}

	currentFP := Fingerprint(data)
	if currentFP == entry.Fingerprint {
		return VerifyResult{
			ID:      id,
			Status:  VerifyOK,
			Stored:  entry.Fingerprint,
			Current: currentFP,
		}
	}

	return VerifyResult{
		ID:      id,
		Status:  VerifyChanged,
		Stored:  entry.Fingerprint,
		Current: currentFP,
	}
}

// VerifyAll checks freshness of all sources in the manifest.
// Returns a slice of results and an error if any source is not OK.
func (l *Lifecycle) VerifyAll() ([]VerifyResult, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var results []VerifyResult
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
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	entry, ok := manifest.Get(id)
	if !ok {
		return nil, fmt.Errorf("source %q not found", id)
	}

	return entry, nil
}

func (l *Lifecycle) GetByURL(url string) (*SourceEntry, error) {
	manifest, err := ReadManifest(l.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	entry, ok := manifest.GetByURL(url)
	if !ok {
		return nil, fmt.Errorf("source with URL %q not found", url)
	}

	return entry, nil
}

func (l *Lifecycle) writeManifest(manifest Manifest) error {
	if l.fileCommitter != nil && l.worktreePath != "" {
		return WriteManifestAndCommit(l.manifestPath, l.worktreePath, manifest, l.fileCommitter)
	}
	return WriteManifest(l.manifestPath, manifest)
}

func (l *Lifecycle) writeCache(id string, data []byte) error {
	if l.fileCommitter != nil && l.worktreePath != "" {
		return WriteCacheAndCommit(l.manifestPath, l.worktreePath, id, data, l.fileCommitter)
	}
	return WriteCache(l.manifestPath, id, data)
}
