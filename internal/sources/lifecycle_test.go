package sources

import (
	"context"
	"errors"
	"testing"
	"time"
)

// MockProvider implements Provider for testing.
type MockProvider struct {
	data []byte
	err  error
}

func (m *MockProvider) Fetch(ctx context.Context, entry SourceEntry) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

func (m *MockProvider) Type() string {
	return "mock"
}

// MockRegistry implements ProviderRegistry for testing.
type MockRegistry struct {
	providers map[string]Provider
}

func (r *MockRegistry) ProviderForType(providerType string) (Provider, error) {
	p, ok := r.providers[providerType]
	if !ok {
		return nil, errors.New("unknown provider type")
	}
	return p, nil
}

func TestLifecycleRegister_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lc := NewLifecycle(dir)

	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "filesystem",
	}

	registered, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if registered.ID != entry.ID {
		t.Errorf("ID mismatch: got %q, want %q", registered.ID, entry.ID)
	}

	// Verify the entry was persisted.
	retrieved, err := lc.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID != entry.ID {
		t.Errorf("retrieved ID mismatch: got %q, want %q", retrieved.ID, entry.ID)
	}
}

func TestLifecycleSync_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register a source first.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "mock",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Sync the source.
	ctx := context.Background()
	result := lc.Sync(ctx, "test-1")

	if result.Error != nil {
		t.Fatalf("Sync failed: %v", result.Error)
	}
	if result.ID != "test-1" {
		t.Errorf("result ID mismatch: got %q, want %q", result.ID, "test-1")
	}
	if result.Fingerprint == "" {
		t.Error("result Fingerprint is empty")
	}

	// Verify the entry was updated with fingerprint and LastSynced.
	retrieved, err := lc.Get("test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Fingerprint != result.Fingerprint {
		t.Errorf("fingerprint mismatch: got %q, want %q", retrieved.Fingerprint, result.Fingerprint)
	}
	if retrieved.LastSynced.IsZero() {
		t.Error("LastSynced is zero")
	}
	if retrieved.SyncFailed {
		t.Error("SyncFailed should be false after successful sync")
	}
}

func TestLifecycleSyncError_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	testErr := errors.New("fetch failed")
	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{err: testErr},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register a source.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "mock",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Sync should fail.
	ctx := context.Background()
	result := lc.Sync(ctx, "test-1")

	if result.Error == nil {
		t.Fatal("expected Sync to fail but it didn't")
	}

	// Verify SyncFailed was set.
	retrieved, err := lc.Get("test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !retrieved.SyncFailed {
		t.Error("SyncFailed should be true after failed sync")
	}
}

func TestLifecycleSyncAll_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register multiple sources.
	for i := 1; i <= 3; i++ {
		id := "test-" + string(rune(48+i))
		entry := SourceEntry{
			ID:           id,
			URL:          "https://example.com/doc" + string(rune(48+i)),
			Title:        "Test Document " + string(rune(48+i)),
			ProviderType: "mock",
		}
		_, err := lc.Register(entry)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
	}

	// Sync all.
	ctx := context.Background()
	results, err := lc.SyncAll(ctx)

	if err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Error != nil {
			t.Errorf("sync result error for %q: %v", result.ID, result.Error)
		}
	}
}

func TestLifecycleVerify_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register and sync a source.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "mock",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()
	syncResult := lc.Sync(ctx, "test-1")
	if syncResult.Error != nil {
		t.Fatalf("Sync failed: %v", syncResult.Error)
	}

	// Verify should report OK.
	verifyResult := lc.Verify("test-1")
	if verifyResult.Status != VerifyOK {
		t.Errorf("expected VerifyOK, got %v", verifyResult.Status)
	}
	if verifyResult.Error != nil {
		t.Errorf("Verify error: %v", verifyResult.Error)
	}
}

func TestLifecycleVerifyStale_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register a source.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "mock",
		SyncFailed:   true, // Simulate a failed sync.
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify should report STALE.
	verifyResult := lc.Verify("test-1")
	if verifyResult.Status != VerifyStale {
		t.Errorf("expected VerifyStale, got %v", verifyResult.Status)
	}
}

func TestLifecycleVerifyMissing_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lc := NewLifecycle(dir)

	// Register a source but don't sync it (no cache).
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "filesystem",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify should report MISSING.
	verifyResult := lc.Verify("test-1")
	if verifyResult.Status != VerifyMissing {
		t.Errorf("expected VerifyMissing, got %v", verifyResult.Status)
	}
}

func TestLifecycleIsFresh_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	// Register and sync a source.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "mock",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()
	syncResult := lc.Sync(ctx, "test-1")
	if syncResult.Error != nil {
		t.Fatalf("Sync failed: %v", syncResult.Error)
	}

	// IsFresh should return true.
	fresh, err := lc.IsFresh("test-1")
	if err != nil {
		t.Fatalf("IsFresh failed: %v", err)
	}
	if !fresh {
		t.Error("expected IsFresh to return true")
	}
}

func TestLifecycleListAll_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lc := NewLifecycle(dir)

	// Register multiple sources.
	for i := 1; i <= 3; i++ {
		id := "test-" + string(rune(48+i))
		entry := SourceEntry{
			ID:           id,
			URL:          "https://example.com/doc" + string(rune(48+i)),
			Title:        "Test Document " + string(rune(48+i)),
			ProviderType: "filesystem",
		}
		_, err := lc.Register(entry)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
	}

	// ListAll should return all sources.
	entries, err := lc.ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestLifecycleGet_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lc := NewLifecycle(dir)

	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		ProviderType: "filesystem",
	}
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Get should return the registered entry.
	retrieved, err := lc.Get("test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID != entry.ID {
		t.Errorf("ID mismatch: got %q, want %q", retrieved.ID, entry.ID)
	}
	if retrieved.URL != entry.URL {
		t.Errorf("URL mismatch: got %q, want %q", retrieved.URL, entry.URL)
	}
}

func TestLifecycleRoundTripJSON_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lc := NewLifecycle(dir)

	// Create a complex entry with all fields populated.
	entry := SourceEntry{
		ID:           "test-1",
		URL:          "https://example.com/doc",
		Title:        "Test Document",
		Fingerprint:  "abc123def456",
		LastSynced:   time.Date(2026, 1, 15, 12, 30, 45, 0, time.UTC),
		ProviderType: "confluence",
		SyncFailed:   false,
	}

	// Register the entry.
	_, err := lc.Register(entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Retrieve it and verify all fields round-trip correctly.
	retrieved, err := lc.Get(entry.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != entry.ID {
		t.Errorf("ID: got %q, want %q", retrieved.ID, entry.ID)
	}
	if retrieved.URL != entry.URL {
		t.Errorf("URL: got %q, want %q", retrieved.URL, entry.URL)
	}
	if retrieved.Title != entry.Title {
		t.Errorf("Title: got %q, want %q", retrieved.Title, entry.Title)
	}
	if retrieved.Fingerprint != entry.Fingerprint {
		t.Errorf("Fingerprint: got %q, want %q", retrieved.Fingerprint, entry.Fingerprint)
	}
	if !retrieved.LastSynced.Equal(entry.LastSynced) {
		t.Errorf("LastSynced: got %v, want %v", retrieved.LastSynced, entry.LastSynced)
	}
	if retrieved.ProviderType != entry.ProviderType {
		t.Errorf("ProviderType: got %q, want %q", retrieved.ProviderType, entry.ProviderType)
	}
	if retrieved.SyncFailed != entry.SyncFailed {
		t.Errorf("SyncFailed: got %v, want %v", retrieved.SyncFailed, entry.SyncFailed)
	}
}
