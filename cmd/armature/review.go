package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Manage conformance reviews for issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newReviewPrepareCmd())
	cmd.AddCommand(newReviewRecordCmd())
	cmd.AddCommand(newReviewCommitsCmd())

	return cmd
}

func newReviewPrepareCmd() *cobra.Command {
	var issueID, base, head, outputFile string

	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare a review bundle for an issue",
		Long: `Prepare a review bundle for an issue by gathering issue metadata and computing the delivery diff.

The bundle is output as JSON to stdout or to a file specified by --output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapReviewError(runReviewPrepare(cmd, issueID, base, head, outputFile))
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID (required)")
	cmd.Flags().StringVar(&base, "base", "", "base revision (required)")
	cmd.Flags().StringVar(&head, "head", "", "head revision (required)")
	cmd.Flags().StringVar(&outputFile, "output", "", "output file (default: stdout)")

	return cmd
}

func runReviewPrepare(cmd *cobra.Command, issueID, base, head, outputFile string) error {
	if issueID == "" {
		return fmt.Errorf("--issue is required")
	}
	if base == "" {
		return fmt.Errorf("--base is required")
	}
	if head == "" {
		return fmt.Errorf("--head is required")
	}

	ctx := currentCtx(cmd)

	// Load snapshot to get issue metadata
	store := newSnapshotStore(ctx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	issuePtr, ok := snap.Issues[issueID]
	if !ok || issuePtr == nil {
		return fmt.Errorf("issue %q not found", issueID)
	}
	issue := *issuePtr

	// Extract title and scope
	title := issue.Title
	scope := issue.Scope

	// Parse acceptance criteria from JSON
	criteria, err := review.ParseAcceptanceCriteria(issue.Acceptance)
	if err != nil {
		return fmt.Errorf("failed to parse acceptance criteria: %w", err)
	}

	// Create git adapter
	git := adapters.New(ctx.RepoPath)

	// Construct the activity log path from the delivery worktree's *actual* git
	// dir, not ctx.RepoPath — ctx.RepoPath is resolved to the parent repo root
	// when this command runs inside a linked worktree (the standard armature
	// delivery flow), while the harness hook writes each worktree's activity log
	// to that worktree's private git dir (<repo>/.git/worktrees/<name>/). Using
	// ctx.RepoPath here would either miss the log entirely or attach an unrelated
	// session's activity as evidence for this delivery. Resolve from the same
	// path the command was invoked against (the --repo flag, defaulting to the
	// current directory, exactly as main.go resolves ctx before the parent-repo
	// walk) so the resolution finds the invoking worktree's own git dir rather
	// than the already-resolved parent repo root.
	invocationPath := invocationRepoPath(cmd)
	binding, err := harnesshook.ResolveBindingFromDir(invocationPath)
	if err != nil {
		return fmt.Errorf("resolve git dir for activity log: %w", err)
	}
	// The file-based binding alone is not the full picture: capture (see
	// resolveIssueBinding below) also falls back to the ARMATURE_ISSUE_ID env var when
	// no armature-issue-id file is present, so an env-bound session's legitimately
	// captured activity would otherwise be silently dropped here (binding.IssueID would
	// come back empty from the file-only resolver even though capture attached activity
	// to this issue). Reuse the same resolution helper capture uses so the prepare-time
	// gate compares against the same binding source that capture used.
	resolvedIssueID := binding.IssueID
	if resolvedIssueID == "" && binding.GitDir != "" {
		resolvedIssueID = resolveIssueBinding(binding.GitDir)
	}
	activityLogPath := ""
	// Only attach the activity log if the binding's issue ID matches the issue being prepared.
	// This prevents a bundle for issue A from carrying issue B's activity log, which would
	// cause downstream record validation to check citations against the wrong issue's evidence.
	if binding.GitDir != "" && resolvedIssueID == issueID {
		activityLogPath = filepath.Join(binding.GitDir, "armature-activity.log")
	}

	// Call prepare — pass real issue metadata (type, outcome, definition of done)
	bundle, err := review.Prepare(git, issueID, title, issue.DefinitionOfDone, issue.Type, issue.Outcome, scope, criteria, base, head, activityLogPath)
	if err != nil {
		return fmt.Errorf("prepare review bundle: %w", err)
	}
	if err := review.AttachGateEvidence(bundle, ctx.IssuesDir); err != nil {
		return fmt.Errorf("attach gate evidence: %w", err)
	}

	// Marshal bundle to JSON
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bundle to JSON: %w", err)
	}

	// Output to file or stdout
	if outputFile != "" {
		if err := os.WriteFile(outputFile, bundleJSON, 0o600); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Review bundle written to %s\n", outputFile)
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(bundleJSON))
	}

	return nil
}

func newReviewRecordCmd() *cobra.Command {
	var issueID, assessmentFile, bundleFile string

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a conformance assessment for an issue",
		Long: `Record a conformance assessment for an issue by reading a ConformanceAssessment
from a JSON file and writing an assessment-attested operation.

The assessment file should contain a valid ConformanceAssessment JSON object.
If the assessment is a duplicate of an existing one (same bundle ID), the command
is idempotent and returns success without writing a duplicate operation.

When --bundle is provided, the command additionally validates that all assessment
citation coordinates reference lines present in the delivery diff, and that the
assessment contract fingerprint matches the bundle contract fingerprint.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapReviewError(runReviewRecord(cmd, issueID, assessmentFile, bundleFile))
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID (required)")
	cmd.Flags().StringVar(&assessmentFile, "assessment", "", "assessment file path or '-' for stdin (required)")
	cmd.Flags().StringVar(&bundleFile, "bundle", "", "review bundle file path (optional; enables diff-index citation validation)")

	return cmd
}

func newReviewCommitsCmd() *cobra.Command {
	var issueID, branch string

	cmd := &cobra.Command{
		Use: "commits [issue-id]",
		Args: func(cmd *cobra.Command, args []string) error {
			return mapReviewError(cobra.MaximumNArgs(1)(cmd, args))
		},
		Short: "List delivery commits for an issue across all conventional-commit types",
		Long: `List delivery commits for an issue by scanning conventional-commit-style commit
messages that reference the issue ID in their scope (e.g., feat(ISSUE-ID): ..., fix(ISSUE-ID): ..., etc.).

This discovers commits across all commit type prefixes (feat, fix, refactor, test, docs, chore),
replacing the coordinator skill's feat-only grep pseudocode which silently dropped other types.

By default, only the currently checked-out branch (HEAD) is scanned. When run from a worktree
whose parent repo has a different branch checked out, or to inspect a task/story branch before
merge, pass --branch, e.g. --branch task/TASK-ID or --branch story/STORY-ID.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if issueID != "" && issueID != args[0] {
					return mapReviewError(fmt.Errorf("conflicting issue ID: positional argument %q and --issue %q disagree", args[0], issueID))
				}
				issueID = args[0]
			}
			return mapReviewError(runReviewCommits(cmd, issueID, branch))
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID (alternative to the positional argument)")
	cmd.Flags().StringVar(&branch, "branch", "HEAD", "branch to scan for commits (e.g. task/TASK-ID or a story branch)")

	return cmd
}

func runReviewCommits(cmd *cobra.Command, issueID, branch string) error {
	if issueID == "" {
		return fmt.Errorf("issue ID is required")
	}

	ctx := currentCtx(cmd)
	git := adapters.New(ctx.RepoPath)

	// When --branch is left at its default ("HEAD"), resolve HEAD against
	// the invocation directory, not ctx.RepoPath — ctx.RepoPath is resolved
	// to the parent repo root when this command runs inside a linked git
	// worktree (the standard `arm claim --worktree` delivery flow), so a
	// bare "HEAD" passed straight through to git.LogBranch would report the
	// *parent* repo's checked-out branch instead of the worktree's own,
	// silently returning wrong (or empty) results with exit 0. Mirrors the
	// same invocation-path resolution runReviewPrepare already applies to
	// the activity log path, for the same underlying reason. Only applies
	// when the flag wasn't explicitly passed — an explicit --branch always
	// means exactly what it says.
	if !cmd.Flags().Changed("branch") {
		if resolved, err := adapters.New(invocationRepoPath(cmd)).CurrentBranch(); err == nil && resolved != "" {
			branch = resolved
		}
	}

	commits, err := review.ReviewCommits(git, issueID, branch)
	if err != nil {
		return fmt.Errorf("failed to list commits for issue %s: %w", issueID, err)
	}

	// Output in JSON format when in agent context, otherwise human-readable
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		data, err := json.Marshal(commits)
		if err != nil {
			return fmt.Errorf("failed to marshal commits: %w", err)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	} else {
		// Human-readable output
		if len(commits) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No commits found for issue %s\n", issueID)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d commit(s) for issue %s:\n\n", len(commits), issueID)
			for _, commit := range commits {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%.7s %s (%s, %s)\n", commit.SHA, commit.Subject, commit.Author, commit.Date)
			}
		}
	}

	return nil
}

func runReviewRecord(cmd *cobra.Command, issueID, assessmentFile, bundleFile string) error {
	if issueID == "" {
		return fmt.Errorf("--issue is required")
	}
	if assessmentFile == "" {
		return fmt.Errorf("--assessment is required")
	}

	if looksLikeJSONArg(bundleFile) {
		if _, err := os.Stat(filepath.Clean(bundleFile)); err != nil {
			return fmt.Errorf("--bundle expects a file path, not JSON content")
		}
	}

	// Decode input: read and parse assessment JSON
	var assessmentData []byte
	var err error
	if assessmentFile == "-" {
		assessmentData, err = io.ReadAll(os.Stdin)
	} else {
		assessmentData, err = os.ReadFile(filepath.Clean(assessmentFile))
	}
	if err != nil {
		return fmt.Errorf("read assessment file: %w", err)
	}

	assessment, err := review.DecodeConformanceAssessment(assessmentData)
	if err != nil {
		return fmt.Errorf("parse assessment JSON: %w", err)
	}

	// Optionally load the review bundle for fingerprint verification and diff-index validation.
	var bundlePtr *review.ReviewBundle
	if bundleFile != "" {
		bundleData, err := os.ReadFile(filepath.Clean(bundleFile))
		if err != nil {
			return fmt.Errorf("read bundle file: %w", err)
		}
		bundle, err := review.DecodeReviewBundle(bundleData)
		if err != nil {
			return fmt.Errorf("parse bundle JSON: %w", err)
		}
		bundlePtr = &bundle
	}

	// Load snapshot to fetch issue data for contract validation and duplicate checking
	ctx := currentCtx(cmd)

	store := newSnapshotStore(ctx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	issuePtr, ok := snap.Issues[issueID]
	if !ok || issuePtr == nil {
		return fmt.Errorf("issue %q not found", issueID)
	}
	issue := *issuePtr

	// Build issue data for the review module
	issueData := &review.IssueData{
		DefinitionOfDone: issue.DefinitionOfDone,
		Scope:            issue.Scope,
		Acceptance:       string(issue.Acceptance),
	}

	// Call the review module to perform all validation and attestation creation
	recordInput := review.RecordInput{
		Assessment: &assessment,
		Bundle:     bundlePtr,
		Issue:      issueData,
		IssueID:    issueID,
	}

	recordResult, err := review.RecordWithDuplicateCheck(recordInput, issue.AssessmentAttestations)
	if err != nil {
		return err
	}

	// Handle idempotent duplicate case
	if recordResult.IsDuplicate {
		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "json" || format == "agent" {
			result := map[string]string{"issue": issueID, "status": "duplicate", "bundle_id": recordResult.Attestation.BundleID}
			data, _ := json.Marshal(result) //nolint:errcheck
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s already recorded (idempotent)\n", recordResult.Attestation.BundleID)
		}
		return nil
	}

	// Render output: create and append the Op
	attJSON, err := json.Marshal(recordResult.Attestation)
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}

	state := mustState(cmd)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	if err != nil {
		return err
	}

	op := ops.Op{
		Type:      ops.OpAssessmentAttested,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Assessment: json.RawMessage(attJSON),
		},
	}

	if err := appendLowStakesOp(state, logPath, op); err != nil {
		return err
	}

	// Output result
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		result := map[string]string{
			"issue":     issueID,
			"status":    "recorded",
			"bundle_id": recordResult.Attestation.BundleID,
			"rating":    recordResult.Attestation.Rating.String(),
		}
		data, _ := json.Marshal(result) //nolint:errcheck
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	} else {
		rating := recordResult.Attestation.Rating.String()
		bundleID := recordResult.Attestation.BundleID
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s recorded with rating %s\n", bundleID, rating)
	}

	return nil
}

const codeReview1 = "REVIEW-1"

func init() {
	armerrors.Register(codeReview1)
}

func looksLikeJSONArg(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func mapReviewError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "--issue is required"),
		strings.Contains(msg, "--base is required"),
		strings.Contains(msg, "--head is required"),
		strings.Contains(msg, "--assessment is required"),
		strings.Contains(msg, "issue ID is required"),
		strings.Contains(msg, "conflicting issue ID"),
		strings.Contains(msg, "accepts at most"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm review --help"}, 2, err)
	case strings.Contains(msg, "read assessment file"),
		strings.Contains(msg, "parse assessment JSON"),
		strings.Contains(msg, "assessment validation failed"),
		strings.Contains(msg, "assessment coverage validation errors"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"jq empty <assessment.json>",
			"arm review record --assessment <assessment.json>",
		}, 1, err)
	case strings.Contains(msg, "failed to list commits"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review commits --issue <issue-id> --branch <reachable-branch>",
		}, 1, err)
	case strings.Contains(msg, "failed to resolve base revision"),
		strings.Contains(msg, "failed to resolve head revision"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review prepare --issue <issue-id> --base <reachable-ref> --head <reachable-ref>",
		}, 1, err)
	case strings.Contains(msg, "not JSON content"),
		strings.Contains(msg, "read bundle file"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review prepare --output <bundle.json>",
			"arm review record --bundle <bundle.json>",
		}, 1, err)
	case strings.Contains(msg, "issue") && strings.Contains(msg, "not found"):
		return armerrors.Wrap(codeReview1, msg, []string{"arm list", "arm show"}, 1, err)
	default:
		return armerrors.Wrap(codeReview1, msg, []string{"arm review prepare --output <bundle.json>"}, 1, err)
	}
}
