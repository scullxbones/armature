package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateRunRecordsEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	head := gitRevParse(t, repo, "HEAD")

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, "full", ev.Profile)
	assert.Equal(t, []string{"true"}, ev.Command)
	assert.Equal(t, head, ev.HeadSHA)
	assert.Equal(t, 0, ev.Exit)
	assert.False(t, ev.Uncommitted, "clean tree must be citable")
	assert.GreaterOrEqual(t, ev.End, ev.Start)
	assert.NotZero(t, ev.Start)
}

func TestGateRunDirtyTreeUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("uncited"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err, "dirty tree still executes the configured command")

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "dirty tree must record the run as uncommitted")
	assert.Equal(t, 0, ev.Exit)
	assert.Equal(t, "full", ev.Profile)
}

func TestGateRunUnconfiguredRepoErrors_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no gates configured")
}

func TestGateRunUnknownProfileErrors(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	_, err := runTrls(t, repo, "gate", "run", "fast")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGateRunEmptyCommandErrors(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {},
	})
	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command")
}

func TestGateRunFailedCommandRecordsEvidence(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"false"},
	})
	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	ev := requireOneGateEvidence(t, repo)
	assert.NotEqual(t, 0, ev.Exit)
	assert.Equal(t, "full", ev.Profile)
}

// TestGateRunUsesInvokingWorktree_REQ_LNGHZN_S10_T3 is the P1 seam: when
// invoked from a linked task worktree, HEAD, cleanliness, and the configured
// command must use that checkout — not the parent repo ResolveContext walks to.
func TestGateRunUsesInvokingWorktree_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"test", "-f", "wt-only"},
	})
	parentHEAD := gitRevParse(t, repo, "HEAD")

	worktreeDir := t.TempDir()
	run(t, repo, "git", "worktree", "add", worktreeDir, "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "wt-only"), []byte("marker"), 0o644))
	run(t, worktreeDir, "git", "add", "wt-only")
	run(t, worktreeDir, "git", "commit", "-m", "feat(task-01): worktree-only marker")
	worktreeHEAD := gitRevParse(t, worktreeDir, "HEAD")
	require.NotEqual(t, parentHEAD, worktreeHEAD, "fixture must diverge parent and worktree HEAD")

	_, err := runTrls(t, worktreeDir, "gate", "run", "full")
	require.NoError(t, err, "command must run in the worktree where wt-only exists")

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, worktreeHEAD, ev.HeadSHA, "evidence must record the invoking worktree HEAD, not the parent")
	assert.NotEqual(t, parentHEAD, ev.HeadSHA)
	assert.Equal(t, 0, ev.Exit)
	assert.False(t, ev.Uncommitted)
}

// TestGateRunDirtiedTreeUncitable_REQ_LNGHZN_S10_T3 covers a clean checkout
// whose configured command leaves a non-ignored file: the run must still be
// recorded uncommitted so evidence cannot cite a tree that no longer matches HEAD.
func TestGateRunDirtiedTreeUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"touch", "generated-by-gate.txt"},
	})

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err, "the configured command itself should succeed")

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "a gate that dirties the tree must not be citable")
	assert.Equal(t, 0, ev.Exit)
	assert.FileExists(t, filepath.Join(repo, "generated-by-gate.txt"))
}

func writeGatesConfig(t *testing.T, repo string, commands map[string][]string) {
	t.Helper()
	path := filepath.Join(repo, ".armature", "config.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	gates := make(map[string]any, len(commands))
	for name, cmd := range commands {
		gates[name] = map[string]any{"command": cmd}
	}
	cfg["gates"] = gates
	out, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func gitRevParse(t *testing.T, repo, rev string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", rev)
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

type gateEvidenceJSON struct {
	Profile     string   `json:"profile"`
	Command     []string `json:"command"`
	HeadSHA     string   `json:"head_sha"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Exit        int      `json:"exit"`
	Uncommitted bool     `json:"uncommitted"`
}

func requireOneGateEvidence(t *testing.T, repo string) gateEvidenceJSON {
	t.Helper()
	found := collectGateEvidence(t, repo)
	require.Len(t, found, 1, "expected exactly one gate-evidence op")
	return found[0]
}

func collectGateEvidence(t *testing.T, repo string) []gateEvidenceJSON {
	t.Helper()
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	var found []gateEvidenceJSON
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(opsDir, entry.Name()))
		require.NoError(t, err)
		for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var arr []json.RawMessage
			if err := json.Unmarshal([]byte(line), &arr); err != nil || len(arr) < 5 {
				continue
			}
			var opType string
			if err := json.Unmarshal(arr[0], &opType); err != nil || opType != "gate-evidence" {
				continue
			}
			var ev gateEvidenceJSON
			require.NoError(t, json.Unmarshal(arr[4], &ev))
			found = append(found, ev)
		}
	}
	return found
}
