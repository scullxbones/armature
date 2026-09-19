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

func TestStaleReviewCmd_NoStaleSources(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo})
	require.NoError(t, root.Execute())

	assert.Contains(t, buf.String(), "No stale sources detected.")
}

func TestStaleReviewCmd_StaleSource_NoCacheFile(t *testing.T) {
	repo := setupRepoWithTask(t)

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
	first := asMap(t, staleSources[0])
	assert.Equal(t, "src-001", first["source_id"])
	assert.Contains(t, first["change_summary"], "no cache found")
}

func TestStaleReviewCmd_StaleSource_FingerprintMismatch(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	originalContent := []byte("original content")
	require.NoError(t, sources.WriteCache(srcDir, "src-002", originalContent))

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
	first := asMap(t, staleSources[0])
	assert.Equal(t, "src-002", first["source_id"])
	assert.Contains(t, first["change_summary"], "fingerprint changed")
}

func TestStaleReviewCmd_MultipleStaleSources(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

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

func TestStaleReviewCmd_StaleSource_WithCitedIssue(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	cacheContent := []byte("some source content")
	require.NoError(t, sources.WriteCache(srcDir, "src-cite-01", cacheContent))

	fp := sources.Fingerprint(cacheContent)
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{
		ID:          "src-cite-01",
		Fingerprint: fp,
	})
	require.NoError(t, sources.WriteManifest(srcDir, m))

	_, err = runTrls(t, repo, "sources", "link",
		"--issue", "task-01",
		"--source-id", "src-cite-01",
	)
	require.NoError(t, err)

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
	first := asMap(t, staleSources[0])
	assert.Equal(t, "src-cite-01", first["source_id"])
	cited := asAnySlice(t, first["cited_issues"])
	assert.NotEmpty(t, cited, "expected task-01 to appear in cited_issues")
}

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
	first := asMap(t, staleSources[0])
	assert.Equal(t, "src-syncfail", first["source_id"])
	assert.Contains(t, first["change_summary"], "last sync failed")
}

func TestStaleReviewCmd_CorruptManifest(t *testing.T) {
	repo := setupRepoWithTask(t)

	issuesDir := filepath.Join(repo, ".armature")
	srcDir := filepath.Join(issuesDir, "sources")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	manifestPath := filepath.Join(srcDir, "manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte("{ invalid json ]"), 0o644))

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
