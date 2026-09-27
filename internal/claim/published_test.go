package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestOwnerPublishedIgnoresUnpublishedOps_REQ_CLAIMORD_W11(t *testing.T) {
	t.Parallel()
	published := []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	}
	unpublished := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	}
	assert.Equal(t, "", OwnerPublished(published, "task-01").Holder)
	assert.True(t, TokenPending(unpublished, "task-01", "tok-a"))
	assert.False(t, TokenPending(published, "task-01", "tok-a"))
}

func TestOwnerPublishedAfterPush_REQ_CLAIMORD_W11(t *testing.T) {
	t.Parallel()
	published := []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}},
	}
	lease := OwnerPublished(published, "task-01")
	assert.Equal(t, "worker-a", lease.Holder)
	assert.Equal(t, "tok-a", lease.Token)
}
