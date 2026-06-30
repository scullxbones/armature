package claim

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestResolveClaimRace_FirstTimestampWins(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestResolveClaimRace_LexicographicTiebreaker(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestIsClaimStale(t *testing.T) {
	// TTL=1 minute = 60 seconds; claimedAt=100, now=161 => stale (100+60=160 < 161)
	assert.True(t, IsClaimStale(100, 0, 1, 161))
	// now=159 => not stale (100+60=160 > 159)
	assert.False(t, IsClaimStale(100, 0, 1, 159))
	// heartbeat at 150, now=209 => not stale (150+60=210 > 209)
	assert.False(t, IsClaimStale(100, 150, 1, 209))
	// heartbeat at 150, now=211 => stale (150+60=210 < 211)
	assert.True(t, IsClaimStale(100, 150, 1, 211))
	// TTL=0 => never stale
	assert.False(t, IsClaimStale(100, 0, 0, 9999))
}

func TestScopeOverlap(t *testing.T) {
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}

// genOp creates an arbitrary ops.Op with a random timestamp and workerID.
func genOp() gopter.Gen {
	return gen.Struct(reflect.TypeOf(ops.Op{}), map[string]gopter.Gen{
		"Type":      gen.Const(ops.OpClaim),
		"TargetID":  gen.Const("task-01"),
		"Timestamp": gen.Int64Range(0, 1000),
		"WorkerID":  gen.OneConstOf("worker-a", "worker-b", "worker-c", "worker-d"),
	})
}

// shuffle returns a copy of the slice with elements in a different order.
func shuffle(claims []ops.Op, rng *rand.Rand) []ops.Op {
	cp := make([]ops.Op, len(claims))
	copy(cp, claims)
	rng.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp
}

// TestPropertyClaimRaceWinnerDeterminism verifies that ResolveClaim always
// picks the same winner regardless of the order in which claims are presented.
func TestPropertyClaimRaceWinnerDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("winner is invariant under permutation", prop.ForAll(
		func(claims []ops.Op) bool {
			if len(claims) == 0 {
				return true
			}
			expected := ResolveClaim(claims)
			// Try a few different shuffles and confirm the winner never changes.
			rng := rand.New(rand.NewSource(42)) // deterministic seed for test reproducibility
			for i := 0; i < 5; i++ {
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

// TestPropertyResolveClaimNoPanic verifies that ResolveClaim never panics
// on arbitrary claim sets including empty slices and single-element slices.
func TestPropertyResolveClaimNoPanic(t *testing.T) {
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

// TestPropertyClaimWinnerMinimality verifies that the winning claim always has
// the minimum timestamp (or lexicographically smallest workerID at equal timestamps),
// which is the key invariant of the race resolution algorithm.
func TestPropertyClaimWinnerMinimality(t *testing.T) {
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
				// No claim should be strictly better than the winner.
				if c.Timestamp < winner.Timestamp {
					return false
				}
				if c.Timestamp == winner.Timestamp && c.WorkerID < winner.WorkerID {
					return false
				}
			}
			return true
		},
		// Use SliceOfN to guarantee at least 1 element, then append arbitrary extras.
		gen.SliceOfN(1, genOp()).FlatMap(func(v interface{}) gopter.Gen {
			base := v.([]ops.Op)
			return gen.SliceOf(genOp()).Map(func(extra []ops.Op) []ops.Op {
				return append(base, extra...)
			})
		}, reflect.TypeOf([]ops.Op{})),
	))

	properties.TestingRun(t)
}

// TestPropertyIsClaimStaleMonotone verifies that once a claim is stale at time T,
// it remains stale at any time T' >= T (monotonicity).
func TestPropertyIsClaimStaleMonotone(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("staleness is monotone in time", prop.ForAll(
		func(claimedAt, lastHeartbeat int64, ttlMinutes int32, now int64) bool {
			if ttlMinutes <= 0 {
				// TTL=0 is always not-stale; no monotonicity to check.
				return true
			}
			if !IsClaimStale(claimedAt, lastHeartbeat, int(ttlMinutes), now) {
				return true // not stale yet, nothing to verify
			}
			// If stale at now, must also be stale at now+delta for any delta >= 0.
			laterNow := now + 1
			return IsClaimStale(claimedAt, lastHeartbeat, int(ttlMinutes), laterNow)
		},
		gen.Int64Range(0, 10000),
		gen.Int64Range(0, 10000),
		gen.Int32Range(1, 100),
		gen.Int64Range(0, 20000),
	))

	properties.TestingRun(t)
}
