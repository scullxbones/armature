package orchestrate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NormalizeScope ---

func TestNormalizeScope_SlashAndClean(t *testing.T) {
	// ToSlash converts backslashes to forward slashes; Clean removes redundancy
	got := orchestrate.NormalizeScope("internal/orchestrate")
	assert.Equal(t, "internal/orchestrate/", got)
}

func TestNormalizeScope_AlreadyClean(t *testing.T) {
	got := orchestrate.NormalizeScope("internal/orchestrate/")
	assert.Equal(t, "internal/orchestrate/", got)
}

func TestNormalizeScope_AddsTrailingSlash(t *testing.T) {
	got := orchestrate.NormalizeScope("internal/orchestrate")
	assert.Equal(t, "internal/orchestrate/", got)
}

func TestNormalizeScope_RemovesDotDot(t *testing.T) {
	got := orchestrate.NormalizeScope("internal/../orchestrate")
	assert.Equal(t, "orchestrate/", got)
}

func TestNormalizeScope_EmptyString(t *testing.T) {
	got := orchestrate.NormalizeScope("")
	// filepath.Clean("") == "." then ToSlash+trailing slash
	assert.Equal(t, "./", got)
}

// TestScopeBoundaryNormalization verifies no HasPrefix false positives after normalizing.
func TestScopeBoundaryNormalization(t *testing.T) {
	scope := orchestrate.NormalizeScope("internal/foo")
	file := "internal/foobar/baz.go"

	// A simple prefix check on raw strings would be a false positive:
	// "internal/foobar/..." starts with "internal/foo" but NOT "internal/foo/"
	assert.False(t, hasPrefix(file, scope),
		"normalized scope with trailing slash prevents false HasPrefix match")

	inScope := orchestrate.NormalizeScope("internal/foobar")
	assert.True(t, hasPrefix(file, inScope),
		"file inside scope is correctly identified")
}

// hasPrefix mirrors the boundary check used in verify.go (exported for test clarity)
func hasPrefix(path, normalizedScope string) bool {
	return len(path) >= len(normalizedScope) && path[:len(normalizedScope)] == normalizedScope
}

// --- stub HarnessAdapter for verify tests ---

type verifyStub struct {
	name   string
	result orchestrate.CheckResult
	err    error
}

func (s *verifyStub) Name() string { return s.name }
func (s *verifyStub) Run(_ context.Context, _ orchestrate.HarnessConfig, _ orchestrate.RunOptions) (orchestrate.CheckResult, error) {
	return s.result, s.err
}

// passAdapter returns a passing check with SeverityInfo.
func passAdapter(name string) orchestrate.HarnessAdapter {
	return &verifyStub{
		name: name,
		result: orchestrate.CheckResult{
			Name:     name,
			Severity: orchestrate.SeverityInfo,
			Passed:   true,
			Message:  "ok",
		},
	}
}

// failAdapter returns a failing check with SeverityError (hard fail).
func failAdapter(name string) orchestrate.HarnessAdapter {
	return &verifyStub{
		name: name,
		result: orchestrate.CheckResult{
			Name:     name,
			Severity: orchestrate.SeverityError,
			Passed:   false,
			Message:  "failed",
		},
	}
}

// warnAdapter returns a failing check with SeverityWarning (soft fail — does NOT stop pipeline).
func warnAdapter(name string) orchestrate.HarnessAdapter {
	return &verifyStub{
		name: name,
		result: orchestrate.CheckResult{
			Name:     name,
			Severity: orchestrate.SeverityWarning,
			Passed:   false,
			Message:  "warning",
		},
	}
}

// --- RunPipeline ---

func TestRunPipeline_AllPass(t *testing.T) {
	adapters := []orchestrate.HarnessAdapter{
		passAdapter("scope-boundary"),
		passAdapter("build"),
		passAdapter("lint"),
	}

	cfg := orchestrate.HarnessConfig{}
	opts := orchestrate.RunOptions{}
	acceptance := json.RawMessage(`["TestFoo passes", "make check green"]`)
	citations := []orchestrate.CitationCheck{{SourceEntryID: "src-1", Accepted: true}}

	state, err := orchestrate.RunPipeline(context.Background(), adapters, cfg, opts, acceptance, citations)
	require.NoError(t, err)
	assert.False(t, state.Failed, "all-pass pipeline should not set Failed")
	// 3 adapter checks + 2 built-in (acceptance-criteria, citations)
	assert.Len(t, state.Checks, 5, "all 3 adapter checks + 2 built-in checks should be recorded")
}

// TestRunPipelineStopsOnHardFail ensures pipeline halts after first SeverityError check.
func TestRunPipelineStopsOnHardFail(t *testing.T) {
	adapters := []orchestrate.HarnessAdapter{
		passAdapter("scope-boundary"),
		failAdapter("build"), // hard fail — stops here
		passAdapter("lint"),  // should never run
	}

	cfg := orchestrate.HarnessConfig{}
	opts := orchestrate.RunOptions{}
	acceptance := json.RawMessage(`["TestFoo passes"]`)
	citations := []orchestrate.CitationCheck{{SourceEntryID: "src-1", Accepted: true}}

	state, err := orchestrate.RunPipeline(context.Background(), adapters, cfg, opts, acceptance, citations)
	require.NoError(t, err)
	assert.True(t, state.Failed, "pipeline with error-severity failure should set Failed")
	// scope-boundary (pass) + build (hard fail) — lint never runs; no built-in checks added
	assert.Len(t, state.Checks, 2, "pipeline should stop after hard fail — lint never runs")
}

// TestRunPipelineDoesNotStopOnWarning ensures SeverityWarning is not a hard fail.
func TestRunPipelineDoesNotStopOnWarning(t *testing.T) {
	adapters := []orchestrate.HarnessAdapter{
		passAdapter("scope-boundary"),
		warnAdapter("build"), // soft fail — continues
		passAdapter("lint"),  // should still run
	}

	cfg := orchestrate.HarnessConfig{}
	opts := orchestrate.RunOptions{}
	acceptance := json.RawMessage(`["TestFoo passes"]`)
	citations := []orchestrate.CitationCheck{{SourceEntryID: "src-1", Accepted: true}}

	state, err := orchestrate.RunPipeline(context.Background(), adapters, cfg, opts, acceptance, citations)
	require.NoError(t, err)
	assert.False(t, state.Failed, "warning-severity failure should not set Failed")
	// 3 adapter checks + 2 built-in (acceptance-criteria, citations)
	assert.Len(t, state.Checks, 5, "all 3 adapter checks + 2 built-in checks should run")
}

// TestRunPipelineRecordsChecksInOrder verifies check names appear in order.
func TestRunPipelineRecordsChecksInOrder(t *testing.T) {
	names := []string{"a", "b", "c"}
	var adapters []orchestrate.HarnessAdapter
	for _, n := range names {
		adapters = append(adapters, passAdapter(n))
	}

	cfg := orchestrate.HarnessConfig{}
	opts := orchestrate.RunOptions{}
	acceptance := json.RawMessage(`["TestFoo passes"]`)
	citations := []orchestrate.CitationCheck{{SourceEntryID: "src-1", Accepted: true}}

	state, err := orchestrate.RunPipeline(context.Background(), adapters, cfg, opts, acceptance, citations)
	require.NoError(t, err)

	// 3 adapter checks + 2 built-in; only verify the first 3 are in order
	require.GreaterOrEqual(t, len(state.Checks), 3)
	for i, name := range names {
		assert.Equal(t, name, state.Checks[i].Name)
	}
}

// TestRunPipelineContextCancellation ensures a cancelled context propagates.
func TestRunPipelineContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	adapters := []orchestrate.HarnessAdapter{
		passAdapter("scope-boundary"),
	}

	cfg := orchestrate.HarnessConfig{}
	opts := orchestrate.RunOptions{}
	acceptance := json.RawMessage(`["TestFoo passes"]`)
	citations := []orchestrate.CitationCheck{}

	_, err := orchestrate.RunPipeline(ctx, adapters, cfg, opts, acceptance, citations)
	assert.Error(t, err, "cancelled context should cause RunPipeline to return an error")
}

// --- CheckCitations ---

// TestCheckCitationsPerSourceEntry ensures citations correlate per SourceEntryID.
func TestCheckCitationsPerSourceEntry(t *testing.T) {
	checks := []orchestrate.CitationCheck{
		{SourceEntryID: "src-1", Accepted: true},
		{SourceEntryID: "src-2", Accepted: false},
		{SourceEntryID: "src-3", Accepted: true},
	}

	result := orchestrate.CheckCitations(checks)
	assert.False(t, result.Passed, "uncited source should cause citation check to fail")
	assert.Contains(t, result.Message, "src-2", "message should name the uncited source")
}

func TestCheckCitationsAllAccepted(t *testing.T) {
	checks := []orchestrate.CitationCheck{
		{SourceEntryID: "src-1", Accepted: true},
		{SourceEntryID: "src-2", Accepted: true},
	}

	result := orchestrate.CheckCitations(checks)
	assert.True(t, result.Passed, "all-accepted citations should pass")
}

func TestCheckCitationsEmpty(t *testing.T) {
	result := orchestrate.CheckCitations(nil)
	assert.True(t, result.Passed, "empty citation list should pass (nothing to verify)")
}

// --- CheckAcceptanceCriteria ---

// TestAcceptanceCriteriaRejectsAllUnverifiable ensures an acceptance array where
// none of the items can be machine-verified is rejected.
func TestAcceptanceCriteriaRejectsAllUnverifiable(t *testing.T) {
	// All items are plain narrative strings — none are machine-verifiable.
	acceptance := json.RawMessage(`[
		"The UI looks good",
		"A human reviewer approves the change"
	]`)

	result := orchestrate.CheckAcceptanceCriteria(acceptance)
	assert.False(t, result.Passed, "all-unverifiable acceptance array should fail")
	assert.Contains(t, result.Message, "unverifiable",
		"message should explain why it was rejected")
}

func TestAcceptanceCriteriaHasMachineVerifiable(t *testing.T) {
	// At least one item is machine-verifiable (contains "passes" or "green").
	acceptance := json.RawMessage(`[
		"The UI looks good",
		"TestFoo passes"
	]`)

	result := orchestrate.CheckAcceptanceCriteria(acceptance)
	assert.True(t, result.Passed, "acceptance with at least one verifiable item should pass")
}

func TestAcceptanceCriteriaEmpty(t *testing.T) {
	// Nil / empty acceptance is rejected — no criteria at all.
	result := orchestrate.CheckAcceptanceCriteria(nil)
	assert.False(t, result.Passed, "nil acceptance should fail")
}

func TestAcceptanceCriteriaEmptyArray(t *testing.T) {
	result := orchestrate.CheckAcceptanceCriteria(json.RawMessage(`[]`))
	assert.False(t, result.Passed, "empty acceptance array should fail")
}

func TestAcceptanceCriteriaMakeCheckGreen(t *testing.T) {
	// "make check green" is the canonical verifiable marker.
	acceptance := json.RawMessage(`["make check green"]`)

	result := orchestrate.CheckAcceptanceCriteria(acceptance)
	assert.True(t, result.Passed, "'make check green' is machine-verifiable")
}

func TestAcceptanceCriteriaPassesKeyword(t *testing.T) {
	// A test name with "passes" at end is verifiable.
	acceptance := json.RawMessage(`["TestRunPipeline passes"]`)

	result := orchestrate.CheckAcceptanceCriteria(acceptance)
	assert.True(t, result.Passed, "item with 'passes' keyword is verifiable")
}
