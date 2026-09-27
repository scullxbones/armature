package materialize

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimStale_EmptyHolderInProgressIsNotExpired_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	issue := &Issue{
		ID:        "FX-STORY",
		Status:    ops.StatusInProgress,
		ClaimedBy: "",
		ClaimTTL:  0,
		ClaimedAt: 0,
	}
	assert.False(t, issue.ClaimStale(1_700_000_000))
}

func TestOwnerMatchesMaterializeGoldenReplay_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixture := filepath.Join(filepath.Dir(thisFile), "..", "claim", "testdata", "golden-real-ops.jsonl")
	data, err := os.ReadFile(fixture)
	require.NoError(t, err)

	var log []ops.Op
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		op, err := ops.ParseLine(line)
		require.NoError(t, err)
		log = append(log, op)
	}
	require.NoError(t, sc.Err())

	state := NewState()
	require.NoError(t, ApplyOpsSorted(state, log))

	seen := map[string]struct{}{}
	var diffs []string
	for _, op := range log {
		if op.TargetID == "" {
			continue
		}
		if _, ok := seen[op.TargetID]; ok {
			continue
		}
		seen[op.TargetID] = struct{}{}
		lease := claim.Owner(log, op.TargetID)
		issue := state.Issues[op.TargetID]
		require.NotNil(t, issue, op.TargetID)
		if issue.ClaimedBy != lease.Holder {
			diffs = append(diffs, op.TargetID+": ClaimedBy="+issue.ClaimedBy+" Owner="+lease.Holder)
		}
		if issue.ClaimToken != lease.Token {
			diffs = append(diffs, op.TargetID+": token mismatch")
		}
	}
	assert.Empty(t, diffs, "golden real-ops fixture ownership diffs (expected none): %v", diffs)
}
