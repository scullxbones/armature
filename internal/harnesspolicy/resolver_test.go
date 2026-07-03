package harnesspolicy

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

func TestResolverLoadsIssuePolicyFromMaterializedState(t *testing.T) {
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

	resolver := NewIssuePolicyResolver(ResolverConfig{
		RepoPath:   repo,
		StateDir:   stateDir,
		SourcesDir: sourcesDir,
	})

	task, err := resolver.Resolve("TASK-1")

	require.NoError(t, err)
	assert.Equal(t, "TASK-1", task.ID)
	assert.Equal(t, "Implement hook", task.Title)
	assert.Equal(t, []string{"internal/harnesshook/"}, task.Scope)
	assert.JSONEq(t, string(acceptance), string(task.Acceptance))
	require.Len(t, task.Citations, 1)
	assert.Equal(t, "SRC-1", task.Citations[0].SourceEntryID)
	assert.True(t, task.Citations[0].Accepted)
}

func TestResolverMarksUnacceptedSourceLinks(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	sourcesDir := filepath.Join(repo, ".armature", "sources")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.MkdirAll(sourcesDir, 0o755))

	manifest := sources.Manifest{}
	manifest.Upsert(sources.SourceEntry{
		ID:           "SRC-2",
		URL:          "docs/design.md",
		Title:        "Design",
		Fingerprint:  "def456",
		LastSynced:   time.Unix(2, 0),
		ProviderType: "filesystem",
	})
	require.NoError(t, sources.WriteManifest(sourcesDir, manifest))

	require.NoError(t, materialize.WriteIssue(issuesDir, materialize.Issue{
		ID:    "TASK-2",
		Title: "Need citation",
		Type:  "task",
		Scope: []string{"internal/harnesspolicy/"},
		SourceLinks: []materialize.SourceLink{
			{SourceEntryID: "SRC-2", SourceURL: "docs/design.md", Title: "Design"},
		},
	}))

	resolver := NewIssuePolicyResolver(ResolverConfig{
		RepoPath:   repo,
		StateDir:   stateDir,
		SourcesDir: sourcesDir,
	})

	task, err := resolver.Resolve("TASK-2")

	require.NoError(t, err)
	require.Len(t, task.Citations, 1)
	assert.Equal(t, "SRC-2", task.Citations[0].SourceEntryID)
	assert.False(t, task.Citations[0].Accepted)
}

func TestResolverTreatsGlobalAcceptanceAsCitingAllSources(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	sourcesDir := filepath.Join(repo, ".armature", "sources")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.MkdirAll(sourcesDir, 0o755))

	manifest := sources.Manifest{}
	manifest.Upsert(sources.SourceEntry{
		ID: "SRC-3", URL: "docs/api.md", Title: "API",
		Fingerprint: "ghi789", LastSynced: time.Unix(3, 0), ProviderType: "filesystem",
	})
	require.NoError(t, sources.WriteManifest(sourcesDir, manifest))

	// arm accept-citation writes CitationAcceptance with empty SourceEntryID
	require.NoError(t, materialize.WriteIssue(issuesDir, materialize.Issue{
		ID:    "TASK-3",
		Title: "Linked source task",
		Type:  "task",
		SourceLinks: []materialize.SourceLink{
			{SourceEntryID: "SRC-3", SourceURL: "docs/api.md", Title: "API"},
		},
		CitationAcceptances: []materialize.CitationAcceptance{
			{WorkerID: "w1", Timestamp: 200, SourceEntryID: ""},
		},
	}))

	resolver := NewIssuePolicyResolver(ResolverConfig{
		RepoPath:   repo,
		StateDir:   stateDir,
		SourcesDir: sourcesDir,
	})

	task, err := resolver.Resolve("TASK-3")

	require.NoError(t, err)
	require.Len(t, task.Citations, 1)
	assert.Equal(t, "SRC-3", task.Citations[0].SourceEntryID)
	assert.True(t, task.Citations[0].Accepted, "global accept-citation should cover all linked sources")
}

func TestResolverRejectsUnknownTask(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0o755))

	resolver := NewIssuePolicyResolver(ResolverConfig{RepoPath: repo, StateDir: stateDir})

	_, err := resolver.Resolve("MISSING")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "task MISSING not found")
}
