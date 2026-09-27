package oporder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
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
	assert.Equal(t, "", claim.OwnerPublished(Ops(Published(locatedA)), "task-01").Holder,
		"unpublished local claim must not be Owner")
	assert.True(t, claim.TokenPending(Ops(Pending(locatedA)), "task-01", "tok-a"))

	parent := t.TempDir()
	gittest.Git(t, parent, "clone", fx.Origin, "b")
	b := filepath.Join(parent, "b")
	gittest.Git(t, b, "checkout", "_armature")
	locatedB, err := LocateOps(LocateInput{OpsWorktree: b, PublishedRef: "origin/_armature"})
	require.NoError(t, err)
	assert.Equal(t, "", claim.OwnerPublished(Ops(Published(locatedB)), "task-01").Holder)
	assert.False(t, claim.TokenPending(Ops(Pending(locatedB)), "task-01", "tok-a"))
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
	lease := claim.OwnerPublished(Ops(Published(locatedA)), "task-01")
	assert.Equal(t, "worker-a", lease.Holder)
	assert.Equal(t, "tok-a", lease.Token)
	assert.Empty(t, Pending(locatedA))

	parent := t.TempDir()
	gittest.Git(t, parent, "clone", fx.Origin, "b")
	b := filepath.Join(parent, "b")
	gittest.Git(t, b, "checkout", "_armature")
	locatedB, err := LocateOps(LocateInput{OpsWorktree: b, PublishedRef: "origin/_armature"})
	require.NoError(t, err)
	leaseB := claim.OwnerPublished(Ops(Published(locatedB)), "task-01")
	assert.Equal(t, "worker-a", leaseB.Holder)
	assert.Equal(t, "tok-a", leaseB.Token)
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
