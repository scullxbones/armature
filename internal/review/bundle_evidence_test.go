package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func attachFromIssuesDir(t *testing.T, bundle *ReviewBundle, issuesDir string) error {
	t.Helper()
	if bundle == nil {
		return fmt.Errorf("attach gate evidence: bundle is nil")
	}
	if issuesDir == "" || bundle.Delivery.HeadSHA == "" {
		return nil
	}
	evs, err := ops.ReadAllGateEvidence(filepath.Join(issuesDir, "ops"))
	if err != nil {
		return fmt.Errorf("read gate evidence: %w", err)
	}
	records, err := json.Marshal(evs)
	if err != nil {
		return fmt.Errorf("read gate evidence: %w", err)
	}
	return AttachGateEvidence(bundle, records)
}

func TestReviewBundleIncludesGateEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()

	issuesDir := t.TempDir()
	opsDir := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ev := ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: head,
		Start:   100,
		End:     101,
		Exit:    0,
	}
	require.NoError(t, ops.AppendGateEvidence(filepath.Join(opsDir, "worker.log"), "worker-1", ev))

	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Issue: IssueInfo{
			ID:    "LNGHZN-S10-T3",
			Type:  "task",
			Title: "Gate evidence",
		},
		Delivery: Delivery{
			HeadSHA: head,
		},
		Fingerprints: Fingerprints{
			Contract: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	require.NoError(t, attachFromIssuesDir(t, bundle, issuesDir))
	require.Len(t, bundle.GateEvidence, 1)
	assert.Equal(t, ev.Profile, bundle.GateEvidence[0].Profile)
	assert.Equal(t, ev.Command, bundle.GateEvidence[0].Command)
	assert.Equal(t, ev.HeadSHA, bundle.GateEvidence[0].HeadSHA)
	assert.Equal(t, ev.Start, bundle.GateEvidence[0].Start)
	assert.Equal(t, ev.End, bundle.GateEvidence[0].End)
	assert.Equal(t, ev.Exit, bundle.GateEvidence[0].Exit)
	assert.Equal(t, mustComputeBundleID(t, *bundle), bundle.BundleID,
		"attach must finalize bundle_id so attestation covers the attached evidence")
}

func TestBundleIDIncludesGateEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()

	base := ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue: IssueInfo{
			ID:      "LNGHZN-S10-T3",
			Type:    "task",
			Title:   "Gate evidence",
			Outcome: "done",
		},
		Contract: Contract{
			DefinitionOfDone: "Evidence is citable",
			Acceptance:       []string{"bundle id binds evidence"},
		},
		Delivery: Delivery{
			BaseSHA:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			HeadSHA:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			ChangedFiles: []string{"cmd/armature/gate.go"},
		},
	}
	withEvidence := base
	withEvidence.GateEvidence = []gateEvidence{{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   100,
		End:     101,
		Exit:    0,
	}}
	otherEvidence := base
	otherEvidence.GateEvidence = []gateEvidence{{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   200,
		End:     201,
		Exit:    0,
	}}

	idNone := mustComputeBundleID(t, base)
	idOne := mustComputeBundleID(t, withEvidence)
	idOther := mustComputeBundleID(t, otherEvidence)

	assert.NotEqual(t, idNone, idOne, "adding gate evidence must change bundle_id")
	assert.NotEqual(t, idOne, idOther, "different gate evidence must change bundle_id")
	assert.Equal(t, idOne, mustComputeBundleID(t, withEvidence), "bundle_id must be deterministic")
}

func attachFixture(t *testing.T, ev ops.GateEvidence, deliveryHead string) *ReviewBundle {
	t.Helper()
	issuesDir := t.TempDir()
	opsDir := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	require.NoError(t, ops.AppendGateEvidence(filepath.Join(opsDir, "worker.log"), "worker-1", ev))
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue: IssueInfo{
			ID:    "LNGHZN-S10-T3",
			Type:  "task",
			Title: "Gate evidence",
		},
		Delivery: Delivery{HeadSHA: deliveryHead},
	}
	require.NoError(t, attachFromIssuesDir(t, bundle, issuesDir))
	return bundle
}

func TestAttachGateEvidence_WrongSHAExcluded_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	bundle := attachFixture(t, ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Start:   1,
		End:     2,
		Exit:    0,
	}, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	assert.Empty(t, bundle.GateEvidence)
}

func TestAttachGateEvidence_UncommittedExcluded_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	bundle := attachFixture(t, ops.GateEvidence{
		Profile:     "full",
		Command:     []string{"true"},
		HeadSHA:     head,
		Start:       1,
		End:         2,
		Exit:        0,
		Uncommitted: true,
	}, head)
	assert.Empty(t, bundle.GateEvidence)
}

func TestAttachGateEvidence_NonZeroExitExcluded_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	bundle := attachFixture(t, ops.GateEvidence{
		Profile: "full",
		Command: []string{"false"},
		HeadSHA: head,
		Start:   1,
		End:     2,
		Exit:    1,
	}, head)
	assert.Empty(t, bundle.GateEvidence)
}

func TestAttachGateEvidence_EmptyDeliveryHeadAttachesNothing_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	bundle := attachFixture(t, ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   1,
		End:     2,
		Exit:    0,
	}, "")
	assert.Empty(t, bundle.GateEvidence)
}

func TestAttachGateEvidence_InvalidPayloadErrors_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	issuesDir := t.TempDir()
	opsDir := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "worker.log"),
		[]byte(`["gate-evidence","full",1,"worker-1","not-an-object"]`+"\n"), 0o644))
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "LNGHZN-S10-T3", Type: "task", Title: "Gate evidence"},
		Delivery:      Delivery{HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}
	err := attachFromIssuesDir(t, bundle, issuesDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestAttachGateEvidence_UnrelatedCorruptJSONStillAttaches_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	issuesDir := t.TempDir()
	opsDir := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ev := ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: head,
		Start:   1,
		End:     2,
		Exit:    0,
	}
	require.NoError(t, ops.AppendGateEvidence(filepath.Join(opsDir, "good.log"), "worker-1", ev))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "junk.log"),
		[]byte("not-json\n{broken\n"), 0o644))
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "LNGHZN-S10-T3", Type: "task", Title: "Gate evidence"},
		Delivery:      Delivery{HeadSHA: head},
	}
	require.NoError(t, attachFromIssuesDir(t, bundle, issuesDir))
	require.Len(t, bundle.GateEvidence, 1)
	assert.Equal(t, ev.Profile, bundle.GateEvidence[0].Profile)
	assert.Equal(t, ev.HeadSHA, bundle.GateEvidence[0].HeadSHA)
}

func TestAttachGateEvidence_CitableMatchFinalizesBundleID_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ev := ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: head,
		Start:   1,
		End:     2,
		Exit:    0,
	}
	bundle := attachFixture(t, ev, head)
	require.Len(t, bundle.GateEvidence, 1)
	assert.Equal(t, ev.Profile, bundle.GateEvidence[0].Profile)
	assert.Equal(t, ev.HeadSHA, bundle.GateEvidence[0].HeadSHA)
	assert.Equal(t, mustComputeBundleID(t, *bundle), bundle.BundleID)
}
