package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/ops"
)

func TestIssueStateRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:     "task-01",
		Type:   "task",
		Status: "open",
		Title:  "Fix auth",
		Parent: "story-01",
		Scope:  []string{"src/auth/**"},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", loaded.ID)
	assert.Equal(t, "Fix auth", loaded.Title)
}

func TestLoadIssue_NormalizesLegacyEmptyEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	issuePath := filepath.Join(dir, "task-01.json")

	raw := []byte(`{
		"id": "task-01",
		"type": "task",
		"status": "open",
		"title": "Fix auth",
		"scope": ["src/auth/**", "", "src/session/**"],
		"context_files": ["docs/design.md", "", "docs/adr.md"]
	}`)
	require.NoError(t, os.WriteFile(issuePath, raw, 0644))

	loaded, err := LoadIssue(issuePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"src/auth/**", "src/session/**"}, loaded.Scope)
	assert.Equal(t, []string{"docs/design.md", "docs/adr.md"}, loaded.ContextFiles)
}

func TestIndexRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")

	index := Index{
		"task-01": IndexEntry{Status: "open", Type: "task", Title: "Fix auth", Parent: "story-01"},
	}

	require.NoError(t, WriteIndex(indexPath, index))

	loaded, err := LoadIndex(indexPath)
	require.NoError(t, err)
	assert.Equal(t, "open", loaded["task-01"].Status)
}

func TestLoadIssueNormalization(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:           "task-normalize",
		Type:         "task",
		Status:       "open",
		Title:        "Fix normalization",
		Scope:        []string{" ", "src/auth/**", "", "src/db/**, src/cache/**"},
		ContextFiles: []string{"", "docs/plan.md", "   "},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-normalize.json"))
	require.NoError(t, err)

	assert.Equal(t, []string{"src/auth/**", "src/db/**", "src/cache/**"}, loaded.Scope)
	assert.Equal(t, []string{"docs/plan.md"}, loaded.ContextFiles)
}

// TestIssueClaimHeldBy_REQ_LNGHZN_S5_T9 is direct unit coverage of the single
// canonical "do I still own this claim?" predicate, ClaimHeldBy. It must be
// true only for an exact workerID+claimToken match while Status is exactly
// ops.StatusClaimed, and false for every other status (including every
// terminal one, which the predicate deliberately folds into "not claimed"
// rather than checking separately — see the method's doc comment), a wrong
// worker, a wrong token, an empty token, and a nil receiver.
func TestIssueClaimHeldBy_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver is never held", func(t *testing.T) {
		t.Parallel()
		var issue *Issue
		assert.False(t, issue.ClaimHeldBy("worker-a", "token-a"))
	})

	t.Run("empty claim token is never held, even if fields happen to match", func(t *testing.T) {
		t.Parallel()
		issue := &Issue{Status: ops.StatusClaimed, ClaimedBy: "worker-a", ClaimToken: ""}
		assert.False(t, issue.ClaimHeldBy("worker-a", ""))
	})

	t.Run("wrong worker is not held", func(t *testing.T) {
		t.Parallel()
		issue := &Issue{Status: ops.StatusClaimed, ClaimedBy: "worker-a", ClaimToken: "token-a"}
		assert.False(t, issue.ClaimHeldBy("worker-b", "token-a"))
	})

	t.Run("wrong token is not held", func(t *testing.T) {
		t.Parallel()
		issue := &Issue{Status: ops.StatusClaimed, ClaimedBy: "worker-a", ClaimToken: "token-a"}
		assert.False(t, issue.ClaimHeldBy("worker-a", "token-b"))
	})

	t.Run("exact worker and token in claimed status is held", func(t *testing.T) {
		t.Parallel()
		issue := &Issue{Status: ops.StatusClaimed, ClaimedBy: "worker-a", ClaimToken: "token-a"}
		assert.True(t, issue.ClaimHeldBy("worker-a", "token-a"))
	})

	for _, status := range []string{
		ops.StatusOpen, ops.StatusInProgress, ops.StatusBlocked, ops.StatusDone, ops.StatusMerged, ops.StatusCancelled,
	} {
		t.Run("not held in status "+status, func(t *testing.T) {
			t.Parallel()
			// ClaimedBy/ClaimToken deliberately still match: only a transition to
			// `open` clears them, so an in-progress or blocked issue can carry a
			// matching worker/token while no longer being "claimed". The predicate
			// must reject on status alone in every one of these cases.
			issue := &Issue{Status: status, ClaimedBy: "worker-a", ClaimToken: "token-a"}
			assert.False(t, issue.ClaimHeldBy("worker-a", "token-a"))
		})
	}
}
