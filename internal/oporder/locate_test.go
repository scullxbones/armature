package oporder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/testexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnpublishedClaimNotOwnedOnRemoteClone_REQ_CLAIMORD_W11(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit mutates process env
	fx := gittest.InitWithOrigin(t)
	a := fx.Dir
	gittest.Git(t, a, "commit", "--allow-empty", "-m", "init ops branch")
	gittest.Git(t, a, "branch", "-M", "_armature")
	gittest.Git(t, a, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	writeOpLog(t, a, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, a, "add", "ops/worker-a.log")
	gittest.Git(t, a, "commit", "-m", "unpublished local claim")

	locatedA, err := LocateOps(LocateInput{OpsWorktree: a, PublishedRef: "origin/_armature"})
	require.NoError(t, err)
	assert.Equal(t, "", claim.Owner(Ops(Published(locatedA)), "task-01").Holder,
		"unpublished local claim must not be Owner")
	assert.True(t, claimTokenPending(Ops(unpublished(locatedA)), "task-01", "tok-a"))

	parent := t.TempDir()
	gittest.Git(t, parent, "clone", fx.Origin, "b")
	b := filepath.Join(parent, "b")
	gittest.Git(t, b, "checkout", "_armature")
	locatedB, err := LocateOps(LocateInput{OpsWorktree: b, PublishedRef: "origin/_armature"})
	require.NoError(t, err)
	assert.Equal(t, "", claim.Owner(Ops(Published(locatedB)), "task-01").Holder)
	assert.False(t, claimTokenPending(Ops(unpublished(locatedB)), "task-01", "tok-a"))
}

func TestPublishedClaimOwnedAfterPush_REQ_CLAIMORD_W11(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit mutates process env
	fx := gittest.InitWithOrigin(t)
	a := fx.Dir
	writeOpLog(t, a, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, a, "add", "ops/worker-a.log")
	gittest.Git(t, a, "commit", "-m", "published claim")
	gittest.Git(t, a, "branch", "-M", "_armature")
	gittest.Git(t, a, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	locatedA, err := LocateOps(LocateInput{OpsWorktree: a, ExtraPublishedTip: "HEAD"})
	require.NoError(t, err)
	lease := claim.Owner(Ops(Published(locatedA)), "task-01")
	assert.Equal(t, "worker-a", lease.Holder)
	assert.Equal(t, "tok-a", lease.Token)
	assert.Empty(t, unpublished(locatedA))

	parent := t.TempDir()
	gittest.Git(t, parent, "clone", fx.Origin, "b")
	b := filepath.Join(parent, "b")
	gittest.Git(t, b, "checkout", "_armature")
	locatedB, err := LocateOps(LocateInput{OpsWorktree: b, PublishedRef: "origin/_armature"})
	require.NoError(t, err)
	leaseB := claim.Owner(Ops(Published(locatedB)), "task-01")
	assert.Equal(t, "worker-a", leaseB.Holder)
	assert.Equal(t, "tok-a", leaseB.Token)
}

func TestLocateOps_MissingFromCommitErrors_REQ_CLAIMORD_W14(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	writeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	})
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "create")

	located, err := LocateOps(LocateInput{
		OpsWorktree:       dir,
		FromCommit:        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		ExtraPublishedTip: "HEAD",
	})
	require.Error(t, err)
	assert.True(t, IsFromCommitMissing(err), "absent checkpoint SHA must not silently replay all history")
	assert.Nil(t, located)

	ok, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD"})
	require.NoError(t, err)
	assert.NotEmpty(t, ok)
}

func writeOpLog(t *testing.T, repo, rel string, log []ops.Op) {
	t.Helper()
	path := filepath.Join(repo, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	var body []byte
	for _, op := range log {
		line, err := ops.MarshalOp(op)
		require.NoError(t, err)
		body = append(body, line...)
		body = append(body, '\n')
	}
	require.NoError(t, os.WriteFile(path, body, 0o644))
}

// TestLocateByCommitWalk_GitInvocationsBounded_REQ_OPORDER_LOCATE proves
// locateByCommitWalk uses a flat git-process budget even as op-changing
// commits grow (CommitWalkReverse + ancestor rev-lists + one ShowFilesBatch),
// not per-SHA CommitterUnix/FirstParent/Diff/IsAncestor or per-file ShowFile.
func TestLocateByCommitWalk_GitInvocationsBounded_REQ_OPORDER_LOCATE(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	countFor := func(nOpCommits int) (calls int, located []LocatedOp) {
		t.Helper()
		fx := gittest.InitWithOrigin(t)
		dir := fx.Dir
		gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
		gittest.Git(t, dir, "branch", "-M", "_armature")
		gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")

		var log []ops.Op
		for i := 0; i < nOpCommits; i++ {
			log = append(log, ops.Op{
				Type: ops.OpCreate, TargetID: fmt.Sprintf("task-%02d", i), Timestamp: int64(100 + i),
				WorkerID: "worker-a", Payload: ops.Payload{Title: "T", NodeType: "task"},
			})
			writeOpLog(t, dir, "ops/worker-a.log", log)
			gittest.Git(t, dir, "add", "ops/worker-a.log")
			gittest.Git(t, dir, "commit", "-m", fmt.Sprintf("op-%d", i))
		}

		counterPath := filepath.Join(t.TempDir(), "git-calls")
		require.NoError(t, os.WriteFile(counterPath, nil, 0o644))
		gc := adapters.New(dir).WithEnviron(countingGitEnviron(t, counterPath))
		out, err := locateByCommitWalk(gc, LocateInput{
			OpsWorktree:       dir,
			ExtraPublishedTip: "HEAD",
			Cutover:           headSHA(t, dir),
		})
		require.NoError(t, err)
		raw, readErr := os.ReadFile(counterPath)
		require.NoError(t, readErr)
		return len(raw), out
	}

	smallCalls, smallLoc := countFor(8)
	largeCalls, largeLoc := countFor(40)
	require.Len(t, smallLoc, 8)
	require.Len(t, largeLoc, 40)
	assert.Equal(t, smallCalls, largeCalls,
		"git exec count must be independent of op-changing commit count (got %d for 8, %d for 40)",
		smallCalls, largeCalls)
	// walk + resolve tip + published rev-list + cutover rev-list + cat-file --batch
	assert.LessOrEqual(t, largeCalls, 8, "locate walk must stay a small constant git budget")
	assert.GreaterOrEqual(t, largeCalls, 4)
}

// TestLocateOps_AdversarialGitConfigMatrix_REQ_OPORDER_LOCATE runs LocateOps
// under hostile local git config (showRoot/quotePath/color/decorate/renames/
// noprefix/relative) and still recovers root + non-ASCII ops paths.
func TestLocateOps_AdversarialGitConfigMatrix_REQ_OPORDER_LOCATE(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	configs := [][][2]string{
		{{"log.showRoot", "false"}, {"core.quotePath", "true"}},
		{
			{"log.showRoot", "false"},
			{"core.quotePath", "true"},
			{"color.ui", "always"},
			{"log.decorate", "short"},
			{"diff.renames", "false"},
			{"diff.noprefix", "true"},
			{"diff.relative", "true"},
		},
	}
	for _, cfg := range configs {
		fx := gittest.InitWithOrigin(t)
		dir := fx.Dir
		nonASCII := "ops/ópś.log"
		writeOpLog(t, dir, nonASCII, []ops.Op{
			{Type: ops.OpCreate, TargetID: "task-root", Timestamp: 50, WorkerID: "ópś",
				Payload: ops.Payload{Title: "R", NodeType: "task"}},
		})
		gittest.Git(t, dir, "add", nonASCII)
		gittest.Git(t, dir, "commit", "-m", "root non-ascii op")
		gittest.Git(t, dir, "branch", "-M", "_armature")
		gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")
		for _, p := range cfg {
			gittest.Git(t, dir, "config", p[0], p[1])
		}
		located, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD"})
		require.NoError(t, err)
		require.NotEmpty(t, located, "root ops must survive log.showRoot=false")
		assert.Equal(t, "task-root", located[0].Op.TargetID)
		assert.Equal(t, "ópś.log", located[0].Seq.Filename)
	}
}

func countingGitEnviron(t *testing.T, counterPath string) []string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(realGit))

	wrapperDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf x >> %q\nexec %q \"$@\"\n", counterPath, realGit)
	testexec.Write(t, filepath.Join(wrapperDir, "git"), []byte(script))

	env := append([]string{}, os.Environ()...)
	pathSet := false
	overlay := wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH")
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") || strings.HasPrefix(e, "Path=") {
			env[i] = "PATH=" + overlay
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH="+overlay)
	}
	return env
}

func unpublished(located []LocatedOp) []LocatedOp {
	var out []LocatedOp
	for _, loc := range located {
		if !loc.Published {
			out = append(out, loc)
		}
	}
	return out
}

func claimTokenPending(pending []ops.Op, issueID, token string) bool {
	if token == "" {
		return false
	}
	for _, op := range pending {
		if op.Type == ops.OpClaim && op.TargetID == issueID && op.Payload.ClaimToken == token {
			return true
		}
	}
	return false
}
