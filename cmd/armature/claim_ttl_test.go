package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/audit"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimTTLZeroAndNegativeRejected_REQ_CLAIMTTL(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err)
	before := claimOpCount(t, logPath)

	_, stderr, err := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", "--ttl", "0")
	require.Error(t, err)
	joined := stderr
	if err != nil {
		joined += err.Error()
	}
	assert.Contains(t, joined, "must be > 0 minutes")
	assert.Contains(t, joined, "ttl 0")

	_, stderr, err = runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", "--ttl", "-5")
	require.Error(t, err)
	joined = stderr + err.Error()
	assert.Contains(t, joined, "must be > 0 minutes")
	assert.Contains(t, joined, "ttl -5")

	assert.Equal(t, before, claimOpCount(t, logPath), "rejected ttl must not append a claim op")
}

func claimOpCount(t *testing.T, logPath string) int {
	t.Helper()
	logged, err := ops.ReadLog(logPath)
	if err != nil {
		return 0
	}
	n := 0
	for _, op := range logged {
		if op.Type == ops.OpClaim {
			n++
		}
	}
	return n
}

func TestRacingClaimsSameOwnerEveryCaller_REQ_CLAIMTTL(t *testing.T) {
	log := []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 110, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "token-b"}},
	}
	owner := claim.Owner(log, "task-01")
	require.Equal(t, "worker-a", owner.Holder)

	state := materialize.NewState()
	require.NoError(t, materialize.ApplyOpsSorted(state, log))
	assert.Equal(t, owner.Holder, state.Issues["task-01"].ClaimedBy)
	assert.Equal(t, owner.Token, state.Issues["task-01"].ClaimToken)

	now := int64(120)
	statusA := foldWorkerStatusFromOwners("worker-a", log, log, 60, now)
	statusB := foldWorkerStatusFromOwners("worker-b", log, log, 60, now)
	assert.Equal(t, "active", statusA.Status)
	assert.Equal(t, "task-01", statusA.ActiveIssue)
	assert.NotEqual(t, "active", statusB.Status)

	var lines []string
	for _, op := range log {
		b, err := ops.MarshalOp(op)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	entries, warnings, err := audit.Load([]audit.Input{{File: "workers.log", Lines: lines}}, audit.Filter{})
	require.NoError(t, err)
	require.Empty(t, warnings)
	for _, e := range entries {
		if e.Type != ops.OpClaim {
			continue
		}
		if e.WorkerID == "worker-a" {
			assert.False(t, e.LostRace)
		}
		if e.WorkerID == "worker-b" {
			assert.True(t, e.LostRace)
		}
	}

	expired := ready.ExpiredClaims(state.Issues, time.Unix(now, 0))
	assert.Empty(t, expired)
	assert.False(t, state.Issues["task-01"].ClaimStale(now))
	assert.True(t, claim.LeaseLive(owner, now))
}

func TestSingleClaimOpConstructorInCmd_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var hits []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := cl.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Op" {
				return true
			}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Type" {
					continue
				}
				if exprMentionsOpClaim(kv.Value) {
					hits = append(hits, name)
				}
			}
			return true
		})
	}
	assert.Empty(t, hits, "cmd production files must construct claim ops only via claim.NewClaimOp, found %v", hits)
}

func exprMentionsOpClaim(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "OpClaim"
}

func TestLoadWorkerLogs_ConcatMatchesListLogFiles_REQ_CLAIMTTL(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	write := func(name string, op ops.Op) {
		t.Helper()
		b, err := ops.MarshalOp(op)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(opsDir, name), append(b, '\n'), 0o644))
	}
	write("z-worker.log", ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "z-worker",
		Payload: ops.Payload{TTL: 60, ClaimToken: "z"},
	})
	write("a-worker.log", ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "a-worker",
		Payload: ops.Payload{TTL: 60, ClaimToken: "a"},
	})

	_, allOps, err := loadWorkerLogs(opsDir)
	require.NoError(t, err)
	require.Len(t, allOps, 2)

	files, err := adapters.ListLogFiles(opsDir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	wantFirst := adapters.WorkerIDFromFilename(files[0])
	assert.Equal(t, wantFirst, allOps[0].WorkerID)

	owner := claim.Owner(allOps, "task-01")
	assert.Equal(t, wantFirst, owner.Holder)
}

func TestLoadWorkerLogs_DropsFilenameWorkerMismatch_REQ_CLAIMTTL(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	write := func(name string, op ops.Op) {
		t.Helper()
		b, err := ops.MarshalOp(op)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(opsDir, name), append(b, '\n'), 0o644))
	}
	write("worker-a~slot-a.log", ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a~slot-a",
		Payload: ops.Payload{TTL: 60, ClaimToken: "real"},
	})
	write("attacker.log", ops.Op{
		Type: ops.OpClaim, TargetID: "task-02", Timestamp: 110, WorkerID: "worker-a~slot-a",
		Payload: ops.Payload{TTL: 60, ClaimToken: "forged"},
	})

	byWorker, allOps, err := loadWorkerLogs(opsDir)
	require.NoError(t, err)
	require.Len(t, allOps, 1)
	assert.Equal(t, "task-01", allOps[0].TargetID)
	assert.NotContains(t, byWorker, "attacker")
	assert.Equal(t, "worker-a~slot-a", claim.Owner(allOps, "task-01").Holder)
	assert.Empty(t, claim.Owner(allOps, "task-02").Holder)
}

func TestResolveClaimAbsent_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	files := []string{
		"internal/claim/claim.go",
		"internal/claim/owner.go",
		"internal/claim/race.go",
		"internal/audit/audit.go",
		"cmd/armature/workers.go",
		"cmd/armature/hook.go",
		"internal/materialize/engine.go",
	}
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err, rel)
		assert.NotContains(t, string(b), "func ResolveClaim", rel)
		assert.NotContains(t, string(b), "zeroTTLHeldLeaseReplayFallbackMinutes", rel)
	}
}
