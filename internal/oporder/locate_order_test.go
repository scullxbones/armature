package oporder

import (
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTwoClonesSkewedClocksExactlyOneOwner_REQ_CLAIMORD_W12(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	a := fx.Dir
	gittest.Git(t, a, "commit", "--allow-empty", "-m", "c0-base")
	gittest.Git(t, a, "branch", "-M", "_armature")
	c0 := headSHA(t, a)

	writeOpLog(t, a, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	})
	gittest.Git(t, a, "add", "ops/worker-a.log")
	gittest.Git(t, a, "commit", "-m", "create")
	gittest.Git(t, a, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	parent := t.TempDir()
	gittest.Git(t, parent, "clone", fx.Origin, "b")
	b := filepath.Join(parent, "b")
	gittest.Git(t, b, "checkout", "_armature")

	writeOpLog(t, b, "ops/worker-b.log", []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100 + 3600, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}},
	})
	gittest.Git(t, b, "add", "ops/worker-b.log")
	gittest.Git(t, b, "commit", "-m", "B claims skewed clock")
	gittest.Git(t, b, "push", "origin", "HEAD:refs/heads/_armature")

	writeOpLog(t, a, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, a, "add", "ops/worker-a.log")
	gittest.Git(t, a, "commit", "-m", "A claims true clock")
	gittest.Git(t, a, "pull", "--rebase", "origin", "_armature")
	gittest.Git(t, a, "push", "origin", "HEAD:refs/heads/_armature")
	gittest.Git(t, b, "pull", "--rebase", "origin", "_armature")

	in := LocateInput{OpsWorktree: a, PublishedRef: "origin/_armature", Cutover: c0}
	locatedA, err := LocateOps(in)
	require.NoError(t, err)
	leaseA := OwnerOf(Published(locatedA), "task-01")
	assert.Equal(t, "worker-b", leaseA.Holder, "first published wins despite A's earlier op.Timestamp")
	assert.Equal(t, "tok-b", leaseA.Token)
	assert.True(t, hasClaimToken(locatedA, "tok-a"), "loser claim remains in JSONL")
	assert.True(t, hasClaimToken(locatedA, "tok-b"))

	locatedB, err := LocateOps(LocateInput{OpsWorktree: b, PublishedRef: "origin/_armature", Cutover: c0})
	require.NoError(t, err)
	leaseB := OwnerOf(Published(locatedB), "task-01")
	assert.Equal(t, "worker-b", leaseB.Holder)
	assert.Equal(t, "tok-b", leaseB.Token)
}

func TestSameSecondPublishTie_REQ_CLAIMORD_W12(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "c0-base")
	gittest.Git(t, dir, "branch", "-M", "_armature")
	c0 := headSHA(t, dir)
	writeOpLog(t, dir, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-a.log")
	gittest.Git(t, dir, "commit", "-m", "A published first same second")
	writeOpLog(t, dir, "ops/worker-b.log", []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-b.log")
	gittest.Git(t, dir, "commit", "-m", "B published second same second")
	gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	located, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD", Cutover: c0})
	require.NoError(t, err)
	lease := OwnerOf(Published(located), "task-01")
	assert.Equal(t, "worker-a", lease.Holder, "commit order, not filename scramble")
	assert.Equal(t, "tok-a", lease.Token)
	located2, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD", Cutover: c0})
	require.NoError(t, err)
	assert.Equal(t, OwnerOf(Published(located2), "task-01"), lease, "rerun stable")
}

func TestGrandfatherPreCutoverTimestampOrder_REQ_CLAIMORD_W12(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, dir, "branch", "-M", "_armature")
	writeOpLog(t, dir, "ops/worker-b.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-b",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-b.log")
	gittest.Git(t, dir, "commit", "-m", "B first publish later clock")
	writeOpLog(t, dir, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-a.log")
	gittest.Git(t, dir, "commit", "-m", "A second publish earlier clock")
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "C0")
	c0 := headSHA(t, dir)
	gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	located, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD", Cutover: c0})
	require.NoError(t, err)
	lease := OwnerOf(Published(located), "task-01")
	assert.Equal(t, "worker-a", lease.Holder, "pre-C0 timestamp Owner matches claim-ttl")
	assert.Equal(t, "tok-a", lease.Token)
}

func TestUnspecifiedCutoverKeepsTimestampOwner_REQ_CLAIMORD_W12(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, dir, "branch", "-M", "_armature")
	writeOpLog(t, dir, "ops/worker-b.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-b",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-b.log")
	gittest.Git(t, dir, "commit", "-m", "B first publish later clock")
	writeOpLog(t, dir, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-a.log")
	gittest.Git(t, dir, "commit", "-m", "A second publish earlier clock")
	gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")

	located, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD"})
	require.NoError(t, err)
	for _, loc := range located {
		assert.Equal(t, 0, loc.Seq.Epoch, "unspecified cutover stays epoch 0")
	}
	lease := OwnerOf(Published(located), "task-01")
	assert.Equal(t, "worker-a", lease.Holder, "historical timestamp Owner must not flip to first-published")
	assert.Equal(t, "tok-a", lease.Token)
}

func TestPersistedCutoverConfigSelectsCommitOrder_REQ_CLAIMORD_W12(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, dir, "branch", "-M", "_armature")
	c0 := headSHA(t, dir)
	writeOpLog(t, dir, "ops/worker-b.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-b",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-b.log")
	gittest.Git(t, dir, "commit", "-m", "B first publish later clock")
	writeOpLog(t, dir, "ops/worker-a.log", []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	})
	gittest.Git(t, dir, "add", "ops/worker-a.log")
	gittest.Git(t, dir, "commit", "-m", "A second publish earlier clock")
	gittest.Git(t, dir, "push", "-u", "origin", "HEAD:refs/heads/_armature")
	require.NoError(t, adapters.New(dir).SetGitConfig(CutoverConfigKey, c0))

	located, err := LocateOps(LocateInput{OpsWorktree: dir, ExtraPublishedTip: "HEAD"})
	require.NoError(t, err)
	lease := OwnerOf(Published(located), "task-01")
	assert.Equal(t, "worker-b", lease.Holder, "persisted C0 uses commit order")
	assert.Equal(t, "tok-b", lease.Token)
}

func headSHA(t *testing.T, repo string) string {
	t.Helper()
	sha, err := adapters.New(repo).HeadSHA()
	require.NoError(t, err)
	return sha
}

func hasClaimToken(located []LocatedOp, token string) bool {
	for _, loc := range located {
		if loc.Op.Type == ops.OpClaim && loc.Op.Payload.ClaimToken == token {
			return true
		}
	}
	return false
}
