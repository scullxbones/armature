package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agentFailureFromStdout(t *testing.T, stdout string) *armerrors.CommandFailure {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON object, got %q", stdout)
	var envelope commandFailureEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	require.NotNil(t, envelope.Error, "stdout must be {error:{...}}, got %s", raw)
	assert.NotEqual(t, armerrors.CodeGeneral1, envelope.Error.Code, "high-traffic paths must not wrap as GENERAL-1")
	assertNoHelpCopOut(t, envelope.Error)
	return envelope.Error
}

func assertNoHelpCopOut(t *testing.T, cf *armerrors.CommandFailure) {
	t.Helper()
	if cf.Code == armerrors.CodeUSAGE || cf.Code == armerrors.CodeGeneral1 {
		return
	}
	for _, action := range cf.NextActions {
		assert.NotContainsf(t, action, "--help",
			"ADR 0020: --help is for USAGE/GENERAL only, code=%s action=%q", cf.Code, action)
	}
}

func TestClaimErrorsCarryNextActions_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr,
		"claim", "--repo", repo, "--issue", "task-01", "--format", "agent")
	assert.Equal(t, 2, code)
	assert.Empty(t, stderr.String(), "Command Failure must not land on stderr")

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, armerrors.CodeUSAGE, cf.Code)
	assert.Contains(t, cf.Cause, "worktree")
	require.NotEmpty(t, cf.NextActions, "claim errors must carry next_actions")
	assert.Contains(t, strings.Join(cf.NextActions, "\n"), "arm claim")

	missing := new(bytes.Buffer)
	code = executeThenHandleRootError(t, missing, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "no-such-issue", "--worktree", "--format", "agent")
	assert.Equal(t, 1, code)
	notFound := agentFailureFromStdout(t, missing.String())
	assert.Equal(t, "CLAIM-1", notFound.Code)
	assert.Contains(t, notFound.Cause, "not found")
	require.NotEmpty(t, notFound.NextActions)
	joined := strings.Join(notFound.NextActions, "\n")
	assert.True(t, strings.Contains(joined, "arm ready") || strings.Contains(joined, "arm list"),
		"next_actions=%v", notFound.NextActions)

	gitMiss := mapClaimError(fmt.Errorf("add worktree: path %s not found", filepath.Join(repo, "missing-wt")))
	var gitCF *armerrors.CommandFailure
	require.ErrorAs(t, gitMiss, &gitCF)
	assert.Equal(t, "CLAIM-1", gitCF.Code)
	assertNoHelpCopOut(t, gitCF)
	gitJoined := strings.Join(gitCF.NextActions, "\n")
	assert.NotContains(t, gitJoined, "arm ready")
	assert.NotContains(t, gitJoined, "arm list")
	assert.True(t, strings.Contains(gitJoined, "arm doctor") || strings.Contains(gitJoined, "arm show"),
		"git/worktree misses must not reuse issue-discovery recovery, next_actions=%v", gitCF.NextActions)

	afterClaim := mapClaimError(fmt.Errorf("issue %s not found after claim", "task-01"))
	var afterCF *armerrors.CommandFailure
	require.ErrorAs(t, afterClaim, &afterCF)
	assert.Equal(t, "CLAIM-1", afterCF.Code)
	assertNoHelpCopOut(t, afterCF)
	afterJoined := strings.Join(afterCF.NextActions, "\n")
	assert.NotContains(t, afterJoined, "arm ready")
	assert.NotContains(t, afterJoined, "arm list")
	assert.True(t, strings.Contains(afterJoined, "arm doctor") || strings.Contains(afterJoined, "arm show"),
		"post-claim rematerialize misses must not reuse issue-discovery recovery, next_actions=%v", afterCF.NextActions)

	extra := new(bytes.Buffer)
	code = executeThenHandleRootError(t, extra, new(bytes.Buffer),
		"claim", "--repo", repo, "a", "b", "c", "--format", "agent")
	assert.Equal(t, 2, code)
	extraCF := agentFailureFromStdout(t, extra.String())
	assert.Equal(t, armerrors.CodeUSAGE, extraCF.Code)
	assert.Contains(t, extraCF.Cause, "accepts at most")
	require.NotEmpty(t, extraCF.NextActions)

	invalidFrom := new(bytes.Buffer)
	code = executeThenHandleRootError(t, invalidFrom, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-01", "--worktree", "--from", repo, "--format", "agent")
	assert.Equal(t, 2, code)
	invalidFromCF := agentFailureFromStdout(t, invalidFrom.String())
	assert.Equal(t, armerrors.CodeUSAGE, invalidFromCF.Code)
	assert.Contains(t, invalidFromCF.Cause, "--from requires an explicit --worktree")
	assert.Equal(t, 2, invalidFromCF.ExitCode)

	invalidIssueID := new(bytes.Buffer)
	code = executeThenHandleRootError(t, invalidIssueID, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "team/task-01", "--worktree", "--format", "agent")
	assert.Equal(t, 2, code)
	invalidIssueIDCF := agentFailureFromStdout(t, invalidIssueID.String())
	assert.Equal(t, armerrors.CodeUSAGE, invalidIssueIDCF.Code)
	assert.Contains(t, invalidIssueIDCF.Cause, "path separators")
	assert.Equal(t, 2, invalidIssueIDCF.ExitCode)

	destination := filepath.Join(t.TempDir(), "new-wt")
	unregisteredFrom := new(bytes.Buffer)
	code = executeThenHandleRootError(t, unregisteredFrom, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-01",
		"--worktree", destination, "--from", filepath.Join(repo, "missing-wt"),
		"--format", "agent")
	assert.Equal(t, 1, code)
	unregisteredFromCF := agentFailureFromStdout(t, unregisteredFrom.String())
	assert.Equal(t, "CLAIM-1", unregisteredFromCF.Code)
	assert.Contains(t, unregisteredFromCF.Cause, "not an existing worktree")
	unregisteredFromActions := strings.Join(unregisteredFromCF.NextActions, "\n")
	assert.Contains(t, unregisteredFromActions, "--from")
	assert.NotContains(t, unregisteredFromActions, "arm doctor")
	assert.NotContains(t, unregisteredFromActions, "arm show")

	detachedParent := filepath.Join(repo, "detached-parent")
	run(t, repo, "git", "worktree", "add", "--detach", detachedParent)
	detachedFrom := new(bytes.Buffer)
	code = executeThenHandleRootError(t, detachedFrom, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-01",
		"--worktree", filepath.Join(t.TempDir(), "detached-child"),
		"--from", detachedParent, "--format", "agent")
	assert.Equal(t, 1, code)
	detachedFromCF := agentFailureFromStdout(t, detachedFrom.String())
	assert.Equal(t, "CLAIM-1", detachedFromCF.Code)
	assert.Contains(t, detachedFromCF.Cause, "must be on a branch")
	detachedFromActions := strings.Join(detachedFromCF.NextActions, "\n")
	assert.Contains(t, detachedFromActions, "--from")
	assert.NotContains(t, detachedFromActions, "arm doctor")
	assert.NotContains(t, detachedFromActions, "arm show")

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-inferred",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            "Inferred task",
			NodeType:         "task",
			Scope:            []string{"cmd/armature/task_inferred.go"},
			DefinitionOfDone: "Task is confirmed",
			Confidence:       "inferred",
		},
	}))
	inferredOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, inferredOut, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-inferred", "--worktree", "--format", "agent")
	assert.Equal(t, 1, code)
	inferredCF := agentFailureFromStdout(t, inferredOut.String())
	assert.Equal(t, "CLAIM-1", inferredCF.Code)
	assert.Contains(t, inferredCF.Cause, "confidence=inferred")
	inferredActions := strings.Join(inferredCF.NextActions, "\n")
	assert.Contains(t, inferredActions, "arm confirm")
	assert.NotContains(t, inferredActions, "arm doctor")
	assert.NotContains(t, inferredActions, "arm show")
}

func TestReviewBundleErrorRemediation_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"review", "record",
		"--repo", repo,
		"--issue", "task-01",
		"--assessment", "assessment.json",
		"--bundle", `{"bundle_id":"not-a-path"}`,
		"--format", "agent")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "TRACEABILITY-1", cf.Code)
	assert.Contains(t, strings.ToLower(cf.Cause), "bundle")
	require.NotEmpty(t, cf.NextActions, "bundle misuse must carry remediation")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm review")
	assert.True(t,
		strings.Contains(joined, "--output") || strings.Contains(joined, "--bundle"),
		"next_actions should point at a file path, got %v", cf.NextActions)

	missingIssue := new(bytes.Buffer)
	code = executeThenHandleRootError(t, missingIssue, new(bytes.Buffer),
		"review", "prepare",
		"--repo", repo,
		"--issue", "no-such-issue",
		"--base", "HEAD",
		"--head", "HEAD",
		"--format", "agent")
	assert.Equal(t, 1, code)
	missing := agentFailureFromStdout(t, missingIssue.String())
	assert.Equal(t, "TRACEABILITY-1", missing.Code)
	assert.Contains(t, missing.Cause, "not found")
	missingJoined := strings.Join(missing.NextActions, "\n")
	assert.True(t, strings.Contains(missingJoined, "arm list") || strings.Contains(missingJoined, "arm show"),
		"missing-issue review errors must point at discovery, next_actions=%v", missing.NextActions)

	assessmentPath := filepath.Join(repo, "assessment.json")
	require.NoError(t, os.WriteFile(assessmentPath, []byte("not-json"), 0o600))
	parseOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, parseOut, new(bytes.Buffer),
		"review", "record",
		"--repo", repo,
		"--issue", "task-01",
		"--assessment", assessmentPath,
		"--format", "agent")
	assert.Equal(t, 1, code)
	parseCF := agentFailureFromStdout(t, parseOut.String())
	assert.Equal(t, "TRACEABILITY-1", parseCF.Code)
	assert.Contains(t, strings.ToLower(parseCF.Cause), "parse")
	parseJoined := strings.Join(parseCF.NextActions, "\n")
	assert.Contains(t, parseJoined, "arm review")
	assert.Contains(t, parseJoined, "jq empty")
	assert.Contains(t, parseJoined, "--assessment")
	assert.NotContains(t, parseJoined, "--output")
	assert.NotContains(t, parseJoined, "--help")

	extraCommits := new(bytes.Buffer)
	code = executeThenHandleRootError(t, extraCommits, new(bytes.Buffer),
		"review", "commits", "--repo", repo, "task-01", "extra", "--format", "agent")
	assert.Equal(t, 2, code)
	extraCF := agentFailureFromStdout(t, extraCommits.String())
	assert.Equal(t, armerrors.CodeUSAGE, extraCF.Code)
	assert.Contains(t, extraCF.Cause, "accepts at most")
	require.NotEmpty(t, extraCF.NextActions)
	assert.Equal(t, 2, extraCF.ExitCode)

	conflictingIssue := new(bytes.Buffer)
	code = executeThenHandleRootError(t, conflictingIssue, new(bytes.Buffer),
		"review", "commits", "--repo", repo, "task-01", "--issue", "task-02", "--format", "agent")
	assert.Equal(t, 2, code)
	conflictingIssueCF := agentFailureFromStdout(t, conflictingIssue.String())
	assert.Equal(t, armerrors.CodeUSAGE, conflictingIssueCF.Code)
	assert.Contains(t, conflictingIssueCF.Cause, "conflicting issue ID")
	assert.Equal(t, 2, conflictingIssueCF.ExitCode)

	validAssessment := filepath.Join(repo, "valid-assessment.json")
	require.NoError(t, os.WriteFile(validAssessment, []byte(`{
		"schema_version":1,
		"bundle_id":"bundle",
		"contract_fingerprint":"contract",
		"delivery_fingerprint":"delivery",
		"results":[{"id":"definition_of_done","status":"satisfied","rationale":"verified","citations":[{"path":"impl.go","line":1}]}]
	}`), 0o600))
	bundleDir, err := os.MkdirTemp(".", "{bundle}")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, os.RemoveAll(bundleDir)) })
	bundlePath := filepath.Join(bundleDir, "bundle.json")
	require.NoError(t, os.WriteFile(bundlePath, []byte("not-json"), 0o600))

	delimitedPath := new(bytes.Buffer)
	code = executeThenHandleRootError(t, delimitedPath, new(bytes.Buffer),
		"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", validAssessment, "--bundle", bundlePath, "--format", "agent")
	assert.Equal(t, 1, code)
	delimitedPathCF := agentFailureFromStdout(t, delimitedPath.String())
	assert.Contains(t, delimitedPathCF.Cause, "parse bundle JSON")
	assert.NotContains(t, delimitedPathCF.Cause, "not JSON content")

	unknownBranch := new(bytes.Buffer)
	code = executeThenHandleRootError(t, unknownBranch, new(bytes.Buffer),
		"review", "commits", "--repo", repo, "--issue", "task-01",
		"--branch", "no-such-branch", "--format", "agent")
	assert.Equal(t, 1, code)
	unknownBranchCF := agentFailureFromStdout(t, unknownBranch.String())
	assert.Equal(t, "TRACEABILITY-1", unknownBranchCF.Code)
	assert.Contains(t, unknownBranchCF.Cause, "failed to list commits")
	unknownBranchActions := strings.Join(unknownBranchCF.NextActions, "\n")
	assert.Contains(t, unknownBranchActions, "--branch <reachable-branch>")
	assert.NotContains(t, unknownBranchActions, "review prepare")
	assert.NotContains(t, unknownBranchActions, "--output")

	emptyResults := filepath.Join(repo, "empty-results-assessment.json")
	require.NoError(t, os.WriteFile(emptyResults, []byte(`{
		"schema_version":1,
		"bundle_id":"bundle",
		"contract_fingerprint":"contract",
		"delivery_fingerprint":"delivery",
		"results":[]
	}`), 0o600))
	emptyResultsOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, emptyResultsOut, new(bytes.Buffer),
		"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", emptyResults, "--format", "agent")
	assert.Equal(t, 1, code)
	emptyResultsCF := agentFailureFromStdout(t, emptyResultsOut.String())
	assert.Equal(t, "TRACEABILITY-1", emptyResultsCF.Code)
	assert.Contains(t, emptyResultsCF.Cause, "assessment validation")
	emptyResultsActions := strings.Join(emptyResultsCF.NextActions, "\n")
	assert.Contains(t, emptyResultsActions, "--assessment")
	assert.Contains(t, emptyResultsActions, "arm review record")
	assert.NotContains(t, emptyResultsActions, "review prepare")
	assert.NotContains(t, emptyResultsActions, "--output")

	coverageMiss := filepath.Join(repo, "coverage-miss-assessment.json")
	coverageFP := review.FingerprintContract(review.Contract{
		DefinitionOfDone: "Task implementation is complete and verified",
		Scope:            []string{"cmd/armature/task_01.go"},
		Acceptance:       []string{},
	})
	require.NoError(t, os.WriteFile(coverageMiss, []byte(fmt.Sprintf(`{
		"schema_version":1,
		"bundle_id":"bundle",
		"contract_fingerprint":%q,
		"delivery_fingerprint":"delivery",
		"results":[{"id":"acceptance[0]","status":"satisfied","rationale":"verified","citations":[{"path":"impl.go","line":1}]}]
	}`, coverageFP)), 0o600))
	coverageOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, coverageOut, new(bytes.Buffer),
		"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", coverageMiss, "--format", "agent")
	assert.Equal(t, 1, code)
	coverageCF := agentFailureFromStdout(t, coverageOut.String())
	assert.Equal(t, "TRACEABILITY-1", coverageCF.Code)
	assert.Contains(t, coverageCF.Cause, "assessment coverage validation errors")
	coverageActions := strings.Join(coverageCF.NextActions, "\n")
	assert.Contains(t, coverageActions, "--assessment")
	assert.Contains(t, coverageActions, "arm review record")
	assert.NotContains(t, coverageActions, "review prepare")
	assert.NotContains(t, coverageActions, "--output")

	unknownBase := new(bytes.Buffer)
	code = executeThenHandleRootError(t, unknownBase, new(bytes.Buffer),
		"review", "prepare", "--repo", repo, "--issue", "task-01",
		"--base", "no-such-base", "--head", "HEAD", "--format", "agent")
	assert.Equal(t, 1, code)
	unknownBaseCF := agentFailureFromStdout(t, unknownBase.String())
	assert.Equal(t, "TRACEABILITY-1", unknownBaseCF.Code)
	assert.Contains(t, unknownBaseCF.Cause, "failed to resolve base revision")
	unknownBaseActions := strings.Join(unknownBaseCF.NextActions, "\n")
	assert.Contains(t, unknownBaseActions, "--base <reachable-ref>")
	assert.Contains(t, unknownBaseActions, "--head <reachable-ref>")
	assert.NotContains(t, unknownBaseActions, "--output")

	unknownHead := new(bytes.Buffer)
	code = executeThenHandleRootError(t, unknownHead, new(bytes.Buffer),
		"review", "prepare", "--repo", repo, "--issue", "task-01",
		"--base", "HEAD", "--head", "no-such-head", "--format", "agent")
	assert.Equal(t, 1, code)
	unknownHeadCF := agentFailureFromStdout(t, unknownHead.String())
	assert.Equal(t, "TRACEABILITY-1", unknownHeadCF.Code)
	assert.Contains(t, unknownHeadCF.Cause, "failed to resolve head revision")
	unknownHeadActions := strings.Join(unknownHeadCF.NextActions, "\n")
	assert.Contains(t, unknownHeadActions, "--base <reachable-ref>")
	assert.Contains(t, unknownHeadActions, "--head <reachable-ref>")
	assert.NotContains(t, unknownHeadActions, "--output")

	opsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.RemoveAll(opsDir))
	require.NoError(t, os.WriteFile(opsDir, []byte("not-a-directory"), 0o600))

	prepareSnapshotOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, prepareSnapshotOut, new(bytes.Buffer),
		"review", "prepare", "--repo", repo, "--issue", "task-01",
		"--base", "HEAD", "--head", "HEAD", "--format", "agent")
	assert.Equal(t, 1, code)
	prepareSnapshotCF := agentFailureFromStdout(t, prepareSnapshotOut.String())
	assert.Equal(t, "TRACEABILITY-1", prepareSnapshotCF.Code)
	assert.Contains(t, prepareSnapshotCF.Cause, "load snapshot")
	prepareSnapshotActions := strings.Join(prepareSnapshotCF.NextActions, "\n")
	assert.Contains(t, prepareSnapshotActions, "arm doctor")
	assert.NotContains(t, prepareSnapshotActions, "review prepare")
	assert.NotContains(t, prepareSnapshotActions, "--output")

	recordSnapshotOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, recordSnapshotOut, new(bytes.Buffer),
		"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", validAssessment, "--format", "agent")
	assert.Equal(t, 1, code)
	recordSnapshotCF := agentFailureFromStdout(t, recordSnapshotOut.String())
	assert.Equal(t, "TRACEABILITY-1", recordSnapshotCF.Code)
	assert.Contains(t, recordSnapshotCF.Cause, "load snapshot")
	recordSnapshotActions := strings.Join(recordSnapshotCF.NextActions, "\n")
	assert.Contains(t, recordSnapshotActions, "arm doctor")
	assert.NotContains(t, recordSnapshotActions, "review prepare")
	assert.NotContains(t, recordSnapshotActions, "--output")
}

func TestTransitionErrorsCarryStructuredCode_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--to", "nope", "--format", "agent")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "TRANSITION-1", cf.Code)
	assert.NotEqual(t, armerrors.CodeGeneral1, cf.Code)
	assert.Contains(t, cf.Cause, "invalid status")
	assert.Contains(t, cf.Cause, "valid values")
	require.NotEmpty(t, cf.NextActions)
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm transition")
	assert.Contains(t, joined, "<valid-status>")
	assert.NotContains(t, joined, "--to done")
	assert.NotContains(t, joined, "--help")

	mistypedMerged := new(bytes.Buffer)
	code = executeThenHandleRootError(t, mistypedMerged, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--to", "merge", "--format", "agent")
	assert.Equal(t, 1, code)
	mistypedCF := agentFailureFromStdout(t, mistypedMerged.String())
	assert.Equal(t, "TRANSITION-1", mistypedCF.Code)
	assert.Contains(t, mistypedCF.Cause, `invalid status "merge"`)
	assert.Contains(t, mistypedCF.Cause, "merged")
	mistypedJoined := strings.Join(mistypedCF.NextActions, "\n")
	assert.Contains(t, mistypedJoined, "<valid-status>")
	assert.NotContains(t, mistypedJoined, "--to done")

	gate := mapTransitionError(fmt.Errorf("delivery gate check failed:\n  1. CleanTree: commit outstanding changes\n\nUse --skip-delivery-gate to override"))
	var gateCF *armerrors.CommandFailure
	require.ErrorAs(t, gate, &gateCF)
	assert.Equal(t, "TRANSITION-1", gateCF.Code)
	assertNoHelpCopOut(t, gateCF)
	gateJoined := strings.Join(gateCF.NextActions, "\n")
	assert.NotContains(t, gateJoined, "--skip-delivery-gate")
	assert.True(t, strings.Contains(gateJoined, "arm doctor") || strings.Contains(gateJoined, "arm show"),
		"delivery-gate recovery must not be the skip flag, next_actions=%v", gateCF.NextActions)

	var round armerrors.CommandFailure
	raw, err := json.Marshal(cf)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &round))
	assert.Equal(t, cf.Code, round.Code)
	assert.Equal(t, cf.Cause, round.Cause)
	assert.Equal(t, cf.NextActions, round.NextActions)
	assert.Equal(t, cf.ExitCode, round.ExitCode)

	missingTo := new(bytes.Buffer)
	code = executeThenHandleRootError(t, missingTo, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--format", "agent")
	assert.Equal(t, 2, code)
	toCF := agentFailureFromStdout(t, missingTo.String())
	assert.Equal(t, armerrors.CodeUSAGE, toCF.Code)
	assert.Contains(t, toCF.Cause, `"to"`)
	require.NotEmpty(t, toCF.NextActions)
	assert.Equal(t, 2, toCF.ExitCode)

	extraArgs := new(bytes.Buffer)
	code = executeThenHandleRootError(t, extraArgs, new(bytes.Buffer),
		"transition", "--repo", repo, "task-01", "extra", "--to", "done", "--format", "agent")
	assert.Equal(t, 2, code)
	extraCF := agentFailureFromStdout(t, extraArgs.String())
	assert.Equal(t, armerrors.CodeUSAGE, extraCF.Code)
	assert.Contains(t, extraCF.Cause, "accepts at most")
	require.NotEmpty(t, extraCF.NextActions)

	run(t, repo, "git", "branch", "-M", "main")
	branchDiscipline := new(bytes.Buffer)
	code = executeThenHandleRootError(t, branchDiscipline, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "verified remediation", "--format", "agent")
	assert.Equal(t, 1, code)
	branchDisciplineCF := agentFailureFromStdout(t, branchDiscipline.String())
	assert.Equal(t, "TRANSITION-1", branchDisciplineCF.Code)
	assert.Contains(t, branchDisciplineCF.Cause, "cannot transition to done")
	assert.Contains(t, strings.Join(branchDisciplineCF.NextActions, "\n"), "--force")
}

func TestReadyAndRenderContextErrorsMapped_REQ_LNGHZN_S6_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	rcOut := new(bytes.Buffer)
	code := executeThenHandleRootError(t, rcOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--issue", "missing-issue", "--format", "agent")
	assert.Equal(t, 1, code)
	rc := agentFailureFromStdout(t, rcOut.String())
	assert.Equal(t, "RENDER-CONTEXT-1", rc.Code)
	assert.Contains(t, rc.Cause, "not found")
	require.NotEmpty(t, rc.NextActions)

	usageOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, usageOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--format", "agent")
	assert.Equal(t, 2, code)
	usage := agentFailureFromStdout(t, usageOut.String())
	assert.Equal(t, armerrors.CodeUSAGE, usage.Code)

	extraRC := new(bytes.Buffer)
	code = executeThenHandleRootError(t, extraRC, new(bytes.Buffer),
		"render-context", "--repo", repo, "a", "b", "--format", "agent")
	assert.Equal(t, 2, code)
	extra := agentFailureFromStdout(t, extraRC.String())
	assert.Equal(t, armerrors.CodeUSAGE, extra.Code)
	assert.Contains(t, extra.Cause, "accepts at most")
	require.NotEmpty(t, extra.NextActions)

	invalidRevisionOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, invalidRevisionOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--issue", "task-01", "--at", "no-such-revision", "--format", "agent")
	assert.Equal(t, 1, code)
	invalidRevision := agentFailureFromStdout(t, invalidRevisionOut.String())
	assert.Equal(t, "RENDER-CONTEXT-1", invalidRevision.Code)
	assert.Contains(t, invalidRevision.Cause, "materialize at no-such-revision")
	invalidRevisionActions := strings.Join(invalidRevision.NextActions, "\n")
	assert.Contains(t, invalidRevisionActions, "--at <reachable-sha>")
	assert.Contains(t, invalidRevisionActions, "arm render-context --issue <issue-id>")

	opsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.RemoveAll(opsDir))
	require.NoError(t, os.WriteFile(opsDir, []byte("not-a-directory"), 0o600))

	readyOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, readyOut, new(bytes.Buffer),
		"ready", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	ready := agentFailureFromStdout(t, readyOut.String())
	assert.Equal(t, "READY-1", ready.Code)
	assert.NotEqual(t, armerrors.CodeGeneral1, ready.Code)
	require.NotEmpty(t, ready.NextActions)
	assert.Contains(t, strings.Join(ready.NextActions, "\n"), "arm doctor")

	snapshotOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, snapshotOut, new(bytes.Buffer),
		"render-context", "--repo", repo, "--issue", "task-01", "--format", "agent")
	assert.Equal(t, 1, code)
	snapshotCF := agentFailureFromStdout(t, snapshotOut.String())
	assert.Equal(t, "RENDER-CONTEXT-1", snapshotCF.Code)
	assert.Contains(t, snapshotCF.Cause, "load snapshot")
	snapshotActions := strings.Join(snapshotCF.NextActions, "\n")
	assert.Contains(t, snapshotActions, "arm doctor")
	assert.NotContains(t, snapshotActions, "arm list")
	assert.NotContains(t, snapshotActions, "arm show")
}
