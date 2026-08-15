package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewBundleIncludesGateEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()

	issuesDir := t.TempDir()
	opsDir := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	ev := ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   100,
		End:     101,
		Exit:    0,
	}
	require.NoError(t, ops.AppendGateEvidence(filepath.Join(opsDir, "worker.log"), "worker-1", ev))

	bundle := &review.ReviewBundle{
		SchemaVersion: review.SchemaVersion,
		BundleID:      "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Issue: review.IssueInfo{
			ID:    "LNGHZN-S10-T3",
			Type:  "task",
			Title: "Gate evidence",
		},
		Fingerprints: review.Fingerprints{
			Contract: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	require.NoError(t, review.AttachGateEvidence(bundle, issuesDir))
	require.Len(t, bundle.GateEvidence, 1)
	assert.Equal(t, ev, bundle.GateEvidence[0])
}
