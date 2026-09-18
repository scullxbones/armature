package claim

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestResolveClaimRace_FirstTimestampWins(t *testing.T) {
	t.Parallel()
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestResolveClaimRace_LexicographicTiebreaker(t *testing.T) {
	t.Parallel()
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestFoldLastActivity(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LastActivity(100), FoldLastActivity(100, 0, 0))
	assert.Equal(t, LastActivity(150), FoldLastActivity(100, 150, 0))
	assert.Equal(t, LastActivity(150), FoldLastActivity(100, 0, 150))
	assert.Equal(t, LastActivity(200), FoldLastActivity(100, 150, 200))
	assert.Equal(t, "unix:150", FoldLastActivity(100, 150, 0).String())
}

func TestIsClaimStale(t *testing.T) {
	t.Parallel()
	assert.True(t, IsClaimStale(FoldLastActivity(100, 0, 0), 1, 161))
	assert.False(t, IsClaimStale(FoldLastActivity(100, 0, 0), 1, 159))
	assert.False(t, IsClaimStale(FoldLastActivity(100, 150, 0), 1, 209))
	assert.True(t, IsClaimStale(FoldLastActivity(100, 150, 0), 1, 211))
	assert.False(t, IsClaimStale(FoldLastActivity(100, 0, 0), 0, 9999))
}

func TestIsClaimStale_ClaimingWorkerActivityExtends(t *testing.T) {
	t.Parallel()
	assert.False(t, IsClaimStale(FoldLastActivity(100, 0, 150), 1, 209))
	assert.True(t, IsClaimStale(FoldLastActivity(100, 0, 150), 1, 211))
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
		"Timestamp": gen.Int64Range(0, 1000),
		"WorkerID":  gen.OneConstOf("worker-a", "worker-b", "worker-c", "worker-d"),
	})
}

func shuffle(claims []ops.Op, rng *rand.Rand) []ops.Op {
	cp := make([]ops.Op, len(claims))
	copy(cp, claims)
	rng.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp
}

func TestPropertyClaimRaceWinnerDeterminism(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("winner is invariant under permutation", prop.ForAll(
		func(claims []ops.Op) bool {
			if len(claims) == 0 {
				return true
			}
			expected := ResolveClaim(claims)
			rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic seed intentional for test reproducibility
			for range 5 {
				shuffled := shuffle(claims, rng)
				got := ResolveClaim(shuffled)
				if got.WorkerID != expected.WorkerID || got.Timestamp != expected.Timestamp {
					return false
				}
			}
			return true
		},
		gen.SliceOf(genOp()),
	))

	properties.TestingRun(t)
}

func TestPropertyResolveClaimNoPanic(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 300
	properties := gopter.NewProperties(parameters)

	properties.Property("no panic on arbitrary claims", prop.ForAll(
		func(claims []ops.Op) bool {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ResolveClaim panicked: %v", r)
				}
			}()
			_ = ResolveClaim(claims)
			return true
		},
		gen.SliceOf(genOp()),
	))

	properties.TestingRun(t)
}

func TestPropertyClaimWinnerMinimality(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("winner has minimum (timestamp, workerID) tuple", prop.ForAll(
		func(claims []ops.Op) bool {
			if len(claims) == 0 {
				return true
			}
			winner := ResolveClaim(claims)
			for _, c := range claims {
				if c.Timestamp < winner.Timestamp {
					return false
				}
				if c.Timestamp == winner.Timestamp && c.WorkerID < winner.WorkerID {
					return false
				}
			}
			return true
		},
		gen.SliceOfN(1, genOp()).FlatMap(func(v any) gopter.Gen {
			base, ok := v.([]ops.Op)
			if !ok {
				return gen.Fail(reflect.TypeFor[[]ops.Op]())
			}
			return gen.SliceOf(genOp()).Map(func(extra []ops.Op) []ops.Op {
				return append(base, extra...)
			})
		}, reflect.TypeFor[[]ops.Op]()),
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
			if ttlMinutes <= 0 {
				return true
			}
			if !IsClaimStale(FoldLastActivity(claimedAt, lastHeartbeat, 0), int(ttlMinutes), now) {
				return true
			}
			laterNow := now + 1
			return IsClaimStale(FoldLastActivity(claimedAt, lastHeartbeat, 0), int(ttlMinutes), laterNow)
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
	found := HasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.False(t, found)
}

func TestHasOverlapDismissalNote_Found(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
		{Type: ops.OpNote, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-01 (same worker, dismissed)"}},
	}
	found := HasOverlapDismissalNote(ops, "task-02", "task-01")
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
	found := HasOverlapDismissalNote(ops, "task-02", "task-01")
	assert.True(t, found)
}

func TestHasOverlapDismissalNote_NotFoundDifferentTarget(t *testing.T) {
	t.Parallel()
	ops := []ops.Op{
		{Type: ops.OpNote, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-02 (same worker, dismissed)"}},
	}
	found := HasOverlapDismissalNote(ops, "task-02", "task-01")
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
