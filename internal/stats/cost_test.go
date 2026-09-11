package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsCost_REQ_TOPTIER_S11_T2(t *testing.T) {
	t.Parallel()

	attJSON, err := json.Marshal(review.AssessmentAttestation{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-a",
		ContractFingerprint: "cf",
		DeliveryFingerprint: "df",
		BaseSHA:             "aa",
		HeadSHA:             "bb",
		ModelIdentity:       "claude-haiku-4-5",
		InputTokens:         1_000_000,
		OutputTokens:        1_000_000,
		Rating:              review.Green,
		ResultFingerprint:   "fp-haiku",
	})
	require.NoError(t, err)

	legacyAttJSON, err := json.Marshal(review.AssessmentAttestation{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-legacy",
		ContractFingerprint: "cf",
		DeliveryFingerprint: "df",
		BaseSHA:             "aa",
		HeadSHA:             "bb",
		Rating:              review.Green,
		ResultFingerprint:   "fp-legacy",
	})
	require.NoError(t, err)

	opList := []ops.Op{
		{
			Type:     ops.OpTransition,
			TargetID: "TASK-A",
			Payload: ops.Payload{
				To:           ops.StatusDone,
				Outcome:      "done with tokens",
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
		},
		{
			Type:     ops.OpAssessmentAttested,
			TargetID: "TASK-B",
			Payload:  ops.Payload{Assessment: attJSON},
		},
		{
			Type:     ops.OpTransition,
			TargetID: "TASK-C",
			Payload:  ops.Payload{To: ops.StatusDone, Outcome: "legacy without tokens"},
		},
		{
			Type:     ops.OpAssessmentAttested,
			TargetID: "TASK-C",
			Payload:  ops.Payload{Assessment: legacyAttJSON},
		},
		{
			Type:     ops.OpTransition,
			TargetID: "TASK-D",
			Payload: ops.Payload{
				To:             ops.StatusDone,
				Outcome:        "overlapping scope",
				PreferredModel: "claude-sonnet-4-5",
				InputTokens:    0,
				OutputTokens:   1_000_000,
			},
		},
	}

	anonAttJSON, err := json.Marshal(review.AssessmentAttestation{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-anon",
		ContractFingerprint: "cf",
		DeliveryFingerprint: "df",
		BaseSHA:             "aa",
		HeadSHA:             "bb",
		InputTokens:         1_000_000,
		OutputTokens:        0,
		Rating:              review.Green,
		ResultFingerprint:   "fp-anon",
	})
	require.NoError(t, err)
	opList = append(opList, ops.Op{
		Type:     ops.OpAssessmentAttested,
		TargetID: "TASK-E",
		Payload:  ops.Payload{Assessment: anonAttJSON},
	})

	usages := CollectUsage(opList)
	require.Len(t, usages, 4, "zero/omitted token fields must not produce usage records")
	assert.Equal(t, "outcome", usages[0].Source)
	assert.Equal(t, "assessment", usages[1].Source)
	assert.Equal(t, "assessment", usages[3].Source)
	assert.Empty(t, usages[3].Model)

	issues := map[string]IssueInfo{
		"STORY-1": {ID: "STORY-1", Type: "story"},
		"STORY-2": {ID: "STORY-2", Type: "story"},
		"TASK-A":  {ID: "TASK-A", Type: "task", Parent: "STORY-1", Scope: []string{"pkg/a.go"}},
		"TASK-B":  {ID: "TASK-B", Type: "task", Parent: "STORY-1", Scope: []string{"pkg/b.go"}},
		"TASK-C":  {ID: "TASK-C", Type: "task", Parent: "STORY-1", Scope: []string{"pkg/c.go"}},
		"TASK-D":  {ID: "TASK-D", Type: "task", Parent: "STORY-2", Scope: []string{"pkg/a.go"}},
		"TASK-E":  {ID: "TASK-E", Type: "task", Parent: "STORY-1", Scope: []string{"pkg/e.go"}, PreferredModel: "claude-haiku-4-5"},
	}

	report := Estimate(usages, issues, DefaultRates())

	require.Len(t, report.Stories, 2)
	byStory := map[string]Totals{}
	for _, s := range report.Stories {
		byStory[s.ID] = s
	}
	// TASK-A: 1M in @ default $3; TASK-B: 1M in + 1M out @ haiku $1/$5
	// TASK-E: assessment without model_identity uses default $3, not issue PreferredModel (haiku $1)
	assert.InDelta(t, 3.00+1.00+5.00+3.00, byStory["STORY-1"].USD, 1e-9)
	assert.Equal(t, 3_000_000, byStory["STORY-1"].InputTokens)
	assert.Equal(t, 1_000_000, byStory["STORY-1"].OutputTokens)
	assert.InDelta(t, 3.00, report.ByIssue["TASK-E"].USD, 1e-9)
	// TASK-D: 1M out @ sonnet $15
	assert.InDelta(t, 15.00, byStory["STORY-2"].USD, 1e-9)

	require.GreaterOrEqual(t, len(report.Waves), 2, "overlapping scopes must not share a wave")
	var waveWithA, waveWithD string
	for _, w := range report.Waves {
		for _, id := range w.Issues {
			if id == "TASK-A" {
				waveWithA = w.ID
			}
			if id == "TASK-D" {
				waveWithD = w.ID
			}
		}
	}
	assert.NotEmpty(t, waveWithA)
	assert.NotEmpty(t, waveWithD)
	assert.NotEqual(t, waveWithA, waveWithD, "TASK-A and TASK-D share scope pkg/a.go")

	storySpend := Rollup(report, "STORY-1", issues)
	assert.InDelta(t, byStory["STORY-1"].USD, storySpend.USD, 1e-9)
	assert.Contains(t, FormatSpend(storySpend), "$12.000000")
}

func TestLoadRateTableOverridesDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "rates.json")
	body := []byte(`{"models":{"default":{"input_usd_per_mtok":10,"output_usd_per_mtok":20}}}`)
	require.NoError(t, os.WriteFile(path, body, 0o600))

	rates, err := LoadRateTable(path)
	require.NoError(t, err)
	assert.Equal(t, 10.0, rates[DefaultModel].InputUSDPerMTok)
	assert.Equal(t, 20.0, rates[DefaultModel].OutputUSDPerMTok)
	assert.Equal(t, 1.0, rates["claude-haiku-4-5"].InputUSDPerMTok, "unmentioned models keep built-in rates")

	usd := USDFromTokens(1_000_000, 1_000_000, RateFor(rates, "", ""))
	assert.InDelta(t, 30.0, usd, 1e-9)
}

func TestResolveRatesPrefersRepoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultRatesFile)
	body := []byte(`{"models":{"default":{"input_usd_per_mtok":7,"output_usd_per_mtok":0}}}`)
	require.NoError(t, os.WriteFile(path, body, 0o600))

	rates, err := ResolveRates("", dir)
	require.NoError(t, err)
	assert.Equal(t, 7.0, rates[DefaultModel].InputUSDPerMTok)

	defaults, err := ResolveRates("", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, DefaultRates()[DefaultModel], defaults[DefaultModel])
}

func TestUSDFromTokensUsesPerMillion(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 3.0, USDFromTokens(1_000_000, 0, DefaultRates()[DefaultModel]), 1e-9)
	assert.InDelta(t, 15.0, USDFromTokens(0, 1_000_000, DefaultRates()[DefaultModel]), 1e-9)
	assert.InDelta(t, 0.0, USDFromTokens(0, 0, DefaultRates()[DefaultModel]), 1e-9)
}

func TestLoadOpsAndRateFallbacks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	owned, err := ops.MarshalOp(ops.Op{
		Type:     ops.OpTransition,
		TargetID: "T1",
		WorkerID: "w",
		Payload:  ops.Payload{InputTokens: 10, OutputTokens: 5},
	})
	require.NoError(t, err)
	foreign, err := ops.MarshalOp(ops.Op{
		Type:     ops.OpTransition,
		TargetID: "T-FOREIGN",
		WorkerID: "other-worker",
		Payload:  ops.Payload{InputTokens: 999, OutputTokens: 999},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "w.log"), append(append(owned, '\n'), append(foreign, '\n')...), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("not a log"), 0o600))

	loaded, err := LoadOps(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 1, "ops whose worker_id mismatches the filename must be excluded")
	assert.Equal(t, "T1", loaded[0].TargetID)
	assert.Equal(t, 10, loaded[0].Payload.InputTokens)

	missing, err := LoadOps(filepath.Join(dir, "no-such-ops"))
	require.NoError(t, err)
	assert.Empty(t, missing)

	notADir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))
	_, err = LoadOps(notADir)
	require.Error(t, err, "unreadable ops dir must not silently understate spend")

	_, err = LoadRateTable(filepath.Join(dir, "missing.json"))
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600))
	_, err = LoadRateTable(filepath.Join(dir, "bad.json"))
	require.Error(t, err)

	flagPath := filepath.Join(dir, "flag.json")
	require.NoError(t, os.WriteFile(flagPath, []byte(`{"models":{"default":{"input_usd_per_mtok":1,"output_usd_per_mtok":1}}}`), 0o600))
	rates, err := ResolveRates(flagPath, "")
	require.NoError(t, err)
	assert.Equal(t, 1.0, rates[DefaultModel].InputUSDPerMTok)

	assert.Equal(t, DefaultRates()[DefaultModel], RateFor(nil, "", ""))
	assert.Equal(t, DefaultRates()["claude-haiku-4-5"], RateFor(DefaultRates(), "claude-haiku-4-5", ""))

	issues := map[string]IssueInfo{
		"A": {ID: "A", Type: "task", Parent: "B"},
		"B": {ID: "B", Type: "task", Parent: "A"},
	}
	assert.NotEmpty(t, StoryRoot("A", issues))
	assert.Equal(t, "missing", StoryRoot("missing", issues))
	assert.False(t, Under("ghost", "STORY", issues))

	usages := CollectUsage([]ops.Op{{
		Type:     ops.OpAssessmentAttested,
		TargetID: "T1",
		Payload:  ops.Payload{Assessment: json.RawMessage(`{`)},
	}})
	assert.Empty(t, usages)
}

func TestLoadOps_UnreadableLogReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// ListLogFiles includes *.log entries that are not directories; a dangling
	// symlink is listed then fails to open, which must not be swallowed.
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "broken.log")))

	_, err := LoadOps(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.log")
}
