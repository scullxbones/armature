package claim

import (
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestFoldLastActivity(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(100), foldLastActivity(100, 0, 0))
	assert.Equal(t, int64(150), foldLastActivity(100, 150, 0))
	assert.Equal(t, int64(150), foldLastActivity(100, 0, 150))
	assert.Equal(t, int64(200), foldLastActivity(100, 150, 200))
}

func TestIsClaimStale(t *testing.T) {
	t.Parallel()
	assert.True(t, isClaimStale(foldLastActivity(100, 0, 0), 1, 160))
	assert.False(t, isClaimStale(foldLastActivity(100, 0, 0), 1, 159))
	assert.False(t, isClaimStale(foldLastActivity(100, 150, 0), 1, 209))
	assert.True(t, isClaimStale(foldLastActivity(100, 150, 0), 1, 210))
	assert.False(t, isClaimStale(foldLastActivity(100, 0, 0), 0, 100+int64(DefaultReplayTTLMinutes)*60-1))
	assert.True(t, isClaimStale(foldLastActivity(100, 0, 0), 0, 100+int64(DefaultReplayTTLMinutes)*60))
}

func TestIsClaimStale_ClaimingWorkerActivityExtends(t *testing.T) {
	t.Parallel()
	assert.False(t, isClaimStale(foldLastActivity(100, 0, 150), 1, 209))
	assert.True(t, isClaimStale(foldLastActivity(100, 0, 150), 1, 210))
}

func TestScopeOverlap(t *testing.T) {
	t.Parallel()
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}

func genOp() gopter.Gen {
	return gen.Struct(reflect.TypeFor[ops.Op](), map[string]gopter.Gen{
		"Type":      gen.Const(ops.OpClaim),
		"TargetID":  gen.Const("task-01"),
		"Timestamp": gen.Int64Range(0, 1000).Map(func(n int64) int64 { return n*1000 + 1 }),
		"WorkerID":  gen.OneConstOf("worker-a", "worker-b", "worker-c", "worker-d"),
	})
}

func TestPropertyOwnerNoPanic(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 300
	properties := gopter.NewProperties(parameters)

	properties.Property("no panic on arbitrary claims", prop.ForAll(
		func(claims []ops.Op) bool {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Owner panicked: %v", r)
				}
			}()
			_ = Owner(claims, "task-01")
			return true
		},
		gen.SliceOf(genOp()),
	))

	properties.TestingRun(t)
}

func TestPropertyAcceptStealsOnlyWhenNotLive(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("foreign steal iff held lease is not live at challenger timestamp", prop.ForAll(
		func(first, second ops.Op) bool {
			first.Type = ops.OpClaim
			second.Type = ops.OpClaim
			first.TargetID = "task-01"
			second.TargetID = "task-01"
			if first.WorkerID == "" {
				first.WorkerID = "worker-a"
			}
			if second.WorkerID == "" {
				second.WorkerID = "worker-b"
			}
			if first.Payload.TTL <= 0 {
				first.Payload.TTL = 1
			}
			held, took := Accept(Lease{}, first)
			if !took {
				return false
			}
			_, stole := Accept(held, second)
			if first.WorkerID == second.WorkerID {
				return stole
			}
			return stole == !LeaseLive(held, second.Timestamp)
		},
		genOp(),
		genOp(),
	))

	properties.TestingRun(t)
}

func TestPropertyIsClaimStaleMonotone(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("staleness is monotone in time", prop.ForAll(
		func(claimedAt, lastHeartbeat int64, ttlMinutes int32, now int64) bool {
			if ttlMinutes < 0 {
				return true
			}
			if !isClaimStale(foldLastActivity(claimedAt, lastHeartbeat, 0), int(ttlMinutes), now) {
				return true
			}
			laterNow := now + 1
			return isClaimStale(foldLastActivity(claimedAt, lastHeartbeat, 0), int(ttlMinutes), laterNow)
		},
		gen.Int64Range(0, 10000),
		gen.Int64Range(0, 10000),
		gen.Int32Range(1, 100),
		gen.Int64Range(0, 20000),
	))

	properties.TestingRun(t)
}

func TestHasOverlapDismissalNote_NotFound(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
		{Type: ops.OpNote, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Some other note"}},
	}
	found := hasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.False(t, found)
}

func TestHasOverlapDismissalNote_Found(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
		{Type: ops.OpNote, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-01 (same worker, dismissed)"}},
	}
	found := hasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.True(t, found)
}

func TestHasOverlapDismissalNote_FoundAmongMultiple(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
		{Type: ops.OpNote, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Some other note"}},
		{Type: ops.OpNote, TargetID: "task-03", Timestamp: 102, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-01 (same worker, dismissed)"}},
		{Type: ops.OpNote, TargetID: "task-02", Timestamp: 103, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-01 (same worker, dismissed)"}},
	}
	found := hasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.True(t, found)
}

func TestHasOverlapDismissalNote_NotFoundDifferentTarget(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpNote, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-02 (same worker, dismissed)"}},
	}
	found := hasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.False(t, found)
}

func TestShouldHeartbeat_NoHeartbeatYet_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	assert.True(t, ShouldHeartbeat(time.Time{}, now))
}

func TestShouldHeartbeat_WithinDebounceWindow_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	lastHeartbeat := now.Add(-2 * time.Minute)
	assert.False(t, ShouldHeartbeat(lastHeartbeat, now))
}

func TestShouldHeartbeat_ExactlyAtDebounceWindow_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	lastHeartbeat := now.Add(-HeartbeatDebounceInterval)
	assert.True(t, ShouldHeartbeat(lastHeartbeat, now))
}

func TestShouldHeartbeat_ExceedsDebounceWindow_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	lastHeartbeat := now.Add(-6 * time.Minute)
	assert.True(t, ShouldHeartbeat(lastHeartbeat, now))
}

func TestShouldHeartbeat_JustBeforeDebounceWindow_REQ_LNGHZN_S3_T1(t *testing.T) {
	t.Parallel()
	now := time.Now()
	lastHeartbeat := now.Add(-HeartbeatDebounceInterval + 100*time.Millisecond)
	assert.False(t, ShouldHeartbeat(lastHeartbeat, now))
}
