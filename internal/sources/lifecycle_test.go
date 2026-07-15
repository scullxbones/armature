package sources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestLifecycleVerifyChanged_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("original content")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	entry := SourceEntry{
		ID:           "changed-1",
		URL:          "https://example.com/doc",
		Title:        "Changing Document",
		ProviderType: "mock",
	}
	if _, err := lc.Register(entry); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if result := lc.Sync(context.Background(), entry.ID); result.Error != nil {
		t.Fatalf("Sync failed: %v", result.Error)
	}

	// Mutate the cache file so its fingerprint diverges from the stored one.
	if err := WriteCache(dir, entry.ID, []byte("tampered content")); err != nil {
		t.Fatalf("WriteCache failed: %v", err)
	}

	result := lc.Verify(entry.ID)
	if result.Status != VerifyChanged {
		t.Fatalf("expected VerifyChanged, got %v", result.Status)
	}
	if result.Stored == "" || result.Current == "" {
		t.Errorf("expected Stored and Current fingerprints populated, got stored=%q current=%q", result.Stored, result.Current)
	}
	if result.Stored == result.Current {
		t.Errorf("expected differing fingerprints, both were %q", result.Stored)
	}
}

func TestLifecycleSyncAll_PartialFailure_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock":    &MockProvider{data: []byte("good content")},
			"failing": &MockProvider{err: errors.New("fetch exploded")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	for id, provider := range map[string]string{"good-1": "mock", "bad-1": "failing"} {
		if _, err := lc.Register(SourceEntry{ID: id, URL: "https://example.com/" + id, Title: id, ProviderType: provider}); err != nil {
			t.Fatalf("Register %s failed: %v", id, err)
		}
	}

	results, err := lc.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("SyncAll with partial success must return nil error, got: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	var failures, successes int
	for _, r := range results {
		if r.Error != nil {
			failures++
		} else {
			successes++
		}
	}
	if failures != 1 || successes != 1 {
		t.Errorf("expected 1 failure and 1 success, got %d failures / %d successes", failures, successes)
	}
}

func TestLifecycleSyncAll_AllFail_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	registry := &MockRegistry{
		providers: map[string]Provider{
			"failing": &MockProvider{err: errors.New("fetch exploded")},
		},
	}
	lc := NewLifecycleWithRegistry(dir, registry)

	for _, id := range []string{"bad-1", "bad-2"} {
		if _, err := lc.Register(SourceEntry{ID: id, URL: "https://example.com/" + id, Title: id, ProviderType: "failing"}); err != nil {
			t.Fatalf("Register %s failed: %v", id, err)
		}
	}

	results, err := lc.SyncAll(context.Background())
	if err == nil {
		t.Fatal("SyncAll with all sources failing must return an error")
	}
	if !strings.Contains(err.Error(), "all sources failed") {
		t.Errorf("error must mention all sources failed, got: %v", err)
	}
	// Per-source detail must be present in the combined error.
	if !strings.Contains(err.Error(), "bad-1") || !strings.Contains(err.Error(), "bad-2") {
		t.Errorf("error must include per-source detail, got: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestLifecycleFilesystemProviderEndToEnd_REQ_ARCHIMP_S18_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	srcFile := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(srcFile, []byte("# local doc\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	// Default registry resolves the real FilesystemProvider.
	lc := NewLifecycle(dir)
	if _, err := lc.Register(SourceEntry{ID: "fs-1", URL: srcFile, Title: "Local Doc", ProviderType: "filesystem"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	result := lc.Sync(context.Background(), "fs-1")
	if result.Error != nil {
		t.Fatalf("Sync via FilesystemProvider failed: %v", result.Error)
	}
	if result.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if result.ProviderType != "filesystem" {
		t.Errorf("expected ProviderType filesystem, got %q", result.ProviderType)
	}

	verify := lc.Verify("fs-1")
	if verify.Status != VerifyOK {
		t.Errorf("expected VerifyOK after sync, got %v", verify.Status)
	}
	content, err := lc.Content("fs-1")
	if err != nil || string(content) != "# local doc\n" {
		t.Errorf("Content mismatch: %q err=%v", content, err)
	}
}

// MockFileCommitter tracks commits for testing.
type MockFileCommitter struct {
	commits []struct {
		relPath string
		message string
	}
	commitErr error
}

func (m *MockFileCommitter) CommitWorktreeOp(relPath, message string) error {
	if m.commitErr != nil {
		return m.commitErr
	}
	m.commits = append(m.commits, struct {
		relPath string
		message string
	}{relPath, message})
	return nil
}

func TestLifecycleRegisterWithAutoCommit_REQ_LNGHZN_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreeDir := t.TempDir()
	fc := &MockFileCommitter{}

	lc := NewLifecycleWithCommitter(dir, &DefaultProviderRegistry{}, worktreeDir, fc)

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

	// Verify manifest.json was committed
	if len(fc.commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(fc.commits))
	}
	if fc.commits[0].message != "sources: update manifest.json" {
		t.Errorf("commit message mismatch: got %q", fc.commits[0].message)
	}
	if !strings.Contains(fc.commits[0].relPath, "manifest.json") {
		t.Errorf("commit relPath should contain manifest.json, got %q", fc.commits[0].relPath)
	}
}

func TestLifecycleSyncAllWithAutoCommit_REQ_LNGHZN_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreeDir := t.TempDir()
	fc := &MockFileCommitter{}

	registry := &MockRegistry{
		providers: map[string]Provider{
			"mock": &MockProvider{data: []byte("test content")},
		},
	}
	lc := NewLifecycleWithCommitter(dir, registry, worktreeDir, fc)

	// Register a source
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

	// Clear the commits from Register
	fc.commits = nil

	// Sync all sources
	ctx := context.Background()
	results, err := lc.SyncAll(ctx)
	if err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}
	if len(results) != 1 || results[0].Error != nil {
		t.Fatalf("SyncAll had unexpected result: %v", results[0].Error)
	}

	// Verify both manifest.json and cache file were committed
	if len(fc.commits) != 2 {
		t.Fatalf("expected 2 commits (manifest + cache), got %d", len(fc.commits))
	}

	hasManifest := false
	hasCache := false
	for _, commit := range fc.commits {
		if strings.Contains(commit.relPath, "manifest.json") {
			hasManifest = true
			if commit.message != "sources: update manifest.json" {
				t.Errorf("manifest commit message mismatch: got %q", commit.message)
			}
		} else if strings.Contains(commit.relPath, ".cache") {
			hasCache = true
			if !strings.Contains(commit.message, "sources: update cache") {
				t.Errorf("cache commit message mismatch: got %q", commit.message)
			}
		}
	}

	if !hasManifest {
		t.Error("expected manifest.json commit")
	}
	if !hasCache {
		t.Error("expected cache file commit")
	}
}

func TestLifecycleNoAutoCommitWhenWorktreePathEmpty_REQ_LNGHZN_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fc := &MockFileCommitter{}

	// Create lifecycle with empty worktreePath (should not commit)
	lc := NewLifecycleWithCommitter(dir, &DefaultProviderRegistry{}, "", fc)

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

	// Verify no commits were made (single-branch mode)
	if len(fc.commits) != 0 {
		t.Fatalf("expected no commits in single-branch mode, got %d", len(fc.commits))
	}
}
