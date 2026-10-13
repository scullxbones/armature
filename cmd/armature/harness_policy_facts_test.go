package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssuePolicyLoaderRejectsUnknownTask(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0o755))

	_, err := newIssuePolicyLoader(repo, stateDir, "").Resolve("MISSING")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "task MISSING not found")
}

func TestIssuePolicyLoaderMapsMaterializedIssueAndSources(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	sourcesDir := filepath.Join(repo, ".armature", "sources")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.MkdirAll(sourcesDir, 0o755))

	manifest := sources.Manifest{}
	manifest.Upsert(sources.SourceEntry{
		ID:           "SRC-1",
		URL:          "docs/spec.md",
		Title:        "Spec",
		Fingerprint:  "abc123",
		LastSynced:   time.Unix(1, 0),
		ProviderType: "filesystem",
	})
	require.NoError(t, sources.WriteManifest(sourcesDir, manifest))

	acceptance := json.RawMessage(`["go test ./... passes"]`)
	require.NoError(t, materialize.WriteIssue(issuesDir, materialize.Issue{
		ID:         "TASK-1",
		Title:      "Implement hook",
		Type:       "task",
		Status:     ops.StatusInProgress,
		Scope:      []string{"internal/harnesshook/"},
		Acceptance: acceptance,
		SourceLinks: []materialize.SourceLink{
			{SourceEntryID: "SRC-1", SourceURL: "docs/spec.md", Title: "Spec"},
		},
		CitationAcceptances: []materialize.CitationAcceptance{
			{WorkerID: "w1", Timestamp: 123, SourceEntryID: "SRC-1"},
		},
	}))

	policy, err := newIssuePolicyLoader(repo, stateDir, sourcesDir).Resolve("TASK-1")

	require.NoError(t, err)
	assert.Equal(t, "TASK-1", policy.ID)
	assert.Equal(t, "Implement hook", policy.Title)
	assert.Equal(t, []string{"internal/harnesshook/"}, policy.Scope)
	assert.JSONEq(t, string(acceptance), string(policy.Acceptance))
	require.Len(t, policy.Citations, 1)
	assert.Equal(t, "SRC-1", policy.Citations[0].SourceEntryID)
	assert.True(t, policy.Citations[0].Accepted)
}
