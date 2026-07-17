package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStaleReviewCmd_NoStaleSources verifies that stale-review exits cleanly
// when no sources are registered.
func TestStaleReviewCmd_NoStaleSources(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo})
	require.NoError(t, root.Execute())

	assert.Contains(t, buf.String(), "No stale sources detected.")
}

// TestStaleReviewCmd_StaleSource_NoCacheFile verifies that stale-review detects
// a source whose cache file is absent and emits it in JSON output.
func TestStaleReviewCmd_StaleSource_NoCacheFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Write a manifest entry with a fingerprint but no corresponding cache file.
	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	entry := sources.SourceEntry{
		ID:          "src-001",
		Fingerprint: "abc123def456",
	}
	m := sources.Manifest{}
	m.Upsert(entry)
	require.NoError(t, sources.WriteManifest(srcDir, m))
	// Deliberately do NOT write a cache file — stale-review should detect data == nil.

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	staleSources, ok := result["stale_sources"].([]any)
	require.True(t, ok, "expected stale_sources array in output")
	require.Len(t, staleSources, 1)
	first := staleSources[0].(map[string]any) //nolint:errcheck // panic in test is acceptable
	assert.Equal(t, "src-001", first["source_id"])
	assert.Contains(t, first["change_summary"], "no cache found")
}

// TestStaleReviewCmd_StaleSource_FingerprintMismatch verifies detection when
// the cached content differs from the stored fingerprint.
func TestStaleReviewCmd_StaleSource_FingerprintMismatch(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	// Write a cache file with content "original".
	originalContent := []byte("original content")
	require.NoError(t, sources.WriteCache(srcDir, "src-002", originalContent))

	// Store a fingerprint that does NOT match the cache content.
	entry := sources.SourceEntry{
		ID:          "src-002",
		Fingerprint: "deadbeefdeadbeef",
	}
	m := sources.Manifest{}
	m.Upsert(entry)
	require.NoError(t, sources.WriteManifest(srcDir, m))

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	staleSources, ok := result["stale_sources"].([]any)
	require.True(t, ok, "expected stale_sources array in output")
	require.Len(t, staleSources, 1)
	first := staleSources[0].(map[string]any) //nolint:errcheck // panic in test is acceptable
	assert.Equal(t, "src-002", first["source_id"])
	assert.Contains(t, first["change_summary"], "fingerprint changed")
}

// TestStaleReviewCmd_MultipleStaleSources verifies that multiple stale entries
// are all reported in the JSON output.
func TestStaleReviewCmd_MultipleStaleSources(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	// Register two entries with no cache files.
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{ID: "src-a", Fingerprint: "fp-a"})
	m.Upsert(sources.SourceEntry{ID: "src-b", Fingerprint: "fp-b"})
	require.NoError(t, sources.WriteManifest(srcDir, m))

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	count, ok := result["count"].(float64)
	require.True(t, ok, "expected count field")
	assert.Equal(t, float64(2), count)
}

// TestStaleReviewCmd_StaleSource_WithCitedIssue verifies that stale-review includes
// cited issue IDs in the output when an issue links to the stale source.
func TestStaleReviewCmd_StaleSource_WithCitedIssue(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	// Write a manifest entry with a valid cache so we can source-link the issue.
	cacheContent := []byte("some source content")
	require.NoError(t, sources.WriteCache(srcDir, "src-cite-01", cacheContent))

	fp := sources.Fingerprint(cacheContent)
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{
		ID:          "src-cite-01",
		Fingerprint: fp,
	})
	require.NoError(t, sources.WriteManifest(srcDir, m))

	// Link task-01 to this source.
	_, err = runTrls(t, repo, "sources", "link",
		"--issue", "task-01",
		"--source-id", "src-cite-01",
	)
	require.NoError(t, err)

	// Now make the source stale by writing a mismatched fingerprint.
	m2 := sources.Manifest{}
	m2.Upsert(sources.SourceEntry{
		ID:          "src-cite-01",
		Fingerprint: "mismatched-fingerprint",
	})
	require.NoError(t, sources.WriteManifest(srcDir, m2))

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	staleSources, ok := result["stale_sources"].([]any)
	require.True(t, ok, "expected stale_sources array")
	require.Len(t, staleSources, 1)
	first := staleSources[0].(map[string]any) //nolint:errcheck
	assert.Equal(t, "src-cite-01", first["source_id"])
	// The cited issues array should include task-01.
	cited, _ := first["cited_issues"].([]any) //nolint:errcheck
	assert.NotEmpty(t, cited, "expected task-01 to appear in cited_issues")
}

// TestStaleReviewCmd_StaleSource_SyncFailed verifies that stale-review surfaces
// sources whose last sync failed (SyncFailed=true), even though Verify()
// short-circuits on SyncFailed before comparing fingerprints. Such sources must
// still be surfaced for review since the upstream may have changed while the
// sync was failing.
func TestStaleReviewCmd_StaleSource_SyncFailed(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	entry := sources.SourceEntry{
		ID:          "src-syncfail",
		Fingerprint: "fp-syncfail",
		SyncFailed:  true,
	}
	m := sources.Manifest{}
	m.Upsert(entry)
	require.NoError(t, sources.WriteManifest(srcDir, m))

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	staleSources, ok := result["stale_sources"].([]any)
	require.True(t, ok, "expected stale_sources array in output")
	require.Len(t, staleSources, 1)
	first := staleSources[0].(map[string]any) //nolint:errcheck // panic in test is acceptable
	assert.Equal(t, "src-syncfail", first["source_id"])
	assert.Contains(t, first["change_summary"], "last sync failed")
}

// TestStaleReviewCmd_CorruptManifest verifies that stale-review fails gracefully
// when the manifest.json file is unreadable or malformed.
func TestStaleReviewCmd_CorruptManifest(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create sources directory and write invalid JSON to manifest.json
	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	// Write corrupted manifest.json with invalid JSON
	manifestPath := filepath.Join(srcDir, "manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte("{ invalid json ]"), 0o644))

	// Run stale-review and expect it to fail
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo})

	err := root.Execute()
	require.Error(t, err, "expected stale-review to fail with corrupt manifest")
	assert.Contains(t, err.Error(), "manifest", "error message should mention manifest issue")
}
