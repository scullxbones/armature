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
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/review"
	"github.com/spf13/cobra"
)

type reviewBundleWriteRow struct {
	Path     string `json:"path"`
	Issue    string `json:"issue"`
	BundleID string `json:"bundle_id"`
}

type reviewAssessmentRow struct {
	Issue    string `json:"issue"`
	Status   string `json:"status"`
	BundleID string `json:"bundle_id"`
	Rating   string `json:"rating,omitempty"`
}

func writeReviewAssessmentEnvelope(cmd *cobra.Command, row reviewAssessmentRow) error {
	help := []string{"arm review commits " + row.Issue + " lists delivery commits for the issue"}
	if row.Status == "duplicate" {
		help = []string{"assessment already recorded for this bundle; no op was appended", help[0]}
	}
	return writeNamedEnvelope(cmd.OutOrStdout(), "assessments", []reviewAssessmentRow{row}, help)
}

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
	cmd.AddCommand(newReviewValidateCmd())
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
		Annotations: output.MarkArtifactOutput(nil, output.ArtifactMode{
			Citation:          output.CitationReviewBundleSchema,
			WhenAllFlagsUnset: []string{"output"},
		}),
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

	title := issue.Title
	scope := issue.Scope

	criteria, err := review.ParseAcceptanceCriteria(issue.Acceptance)
	if err != nil {
		return fmt.Errorf("failed to parse acceptance criteria: %w", err)
	}

	git := adapters.New(ctx.RepoPath)

	invocationPath := invocationRepoPath(cmd)
	binding, err := harnesshook.ResolveBindingFromDir(invocationPath)
	if err != nil {
		return fmt.Errorf("resolve git dir for activity log: %w", err)
	}
	resolvedIssueID := binding.IssueID
	if resolvedIssueID == "" && binding.GitDir != "" {
		resolvedIssueID = issueBindingFromGitDirOrEnv(binding.GitDir)
	}
	activityLogPath := ""
	if binding.GitDir != "" && resolvedIssueID == issueID {
		activityLogPath = filepath.Join(binding.GitDir, "armature-activity.log")
	}

	bundle, err := review.Prepare(git, issueID, title, issue.DefinitionOfDone, issue.Type, issue.Outcome, scope, criteria, base, head, activityLogPath)
	if err != nil {
		return fmt.Errorf("prepare review bundle: %w", err)
	}
	if err := review.AttachGateEvidence(bundle, ctx.IssuesDir); err != nil {
		return fmt.Errorf("attach gate evidence: %w", err)
	}

	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bundle to JSON: %w", err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, bundleJSON, 0o600); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		if structuredFormat(cmd) {
			row := reviewBundleWriteRow{Path: outputFile, Issue: issueID, BundleID: bundle.BundleID}
			help := []string{
				"arm review record --issue " + issueID + " --assessment <assessment.json> --bundle " + outputFile,
			}
			return writeNamedEnvelope(cmd.OutOrStdout(), "bundles", []reviewBundleWriteRow{row}, help)
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

	if !cmd.Flags().Changed("branch") {
		if resolved, err := adapters.New(invocationRepoPath(cmd)).CurrentBranch(); err == nil && resolved != "" {
			branch = resolved
		}
	}

	commits, err := review.ReviewCommits(git, issueID, branch)
	if err != nil {
		return fmt.Errorf("failed to list commits for issue %s: %w", issueID, err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		help := []string{"arm review prepare --issue " + issueID + " --base <sha> --head <sha> builds a review bundle"}
		if len(commits) == 0 {
			help = []string{"no delivery commits found for " + issueID, help[0]}
		}
		return writeNamedEnvelope(cmd.OutOrStdout(), "commits", commits, help)
	}

	if len(commits) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No commits found for issue %s\n", issueID)
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d commit(s) for issue %s:\n\n", len(commits), issueID)
		for _, commit := range commits {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%.7s %s (%s, %s)\n", commit.SHA, commit.Subject, commit.Author, commit.Date)
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

	assessmentData, err := readAssessmentFile(assessmentFile)
	if err != nil {
		return err
	}

	assessment, err := review.DecodeConformanceAssessment(assessmentData)
	if err != nil {
		return fmt.Errorf("parse assessment JSON: %w", err)
	}

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

	issueData := &review.IssueData{
		DefinitionOfDone: issue.DefinitionOfDone,
		Scope:            issue.Scope,
		Acceptance:       string(issue.Acceptance),
	}

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

	if recordResult.IsDuplicate {
		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "json" || format == "agent" {
			return writeReviewAssessmentEnvelope(cmd, reviewAssessmentRow{
				Issue:    issueID,
				Status:   "duplicate",
				BundleID: recordResult.Attestation.BundleID,
			})
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s already recorded (idempotent)\n", recordResult.Attestation.BundleID)
		return nil
	}

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

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		return writeReviewAssessmentEnvelope(cmd, reviewAssessmentRow{
			Issue:    issueID,
			Status:   "recorded",
			BundleID: recordResult.Attestation.BundleID,
			Rating:   recordResult.Attestation.Rating.String(),
		})
	}
	rating := recordResult.Attestation.Rating.String()
	bundleID := recordResult.Attestation.BundleID
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s recorded with rating %s\n", bundleID, rating)
	return nil
}

const codeReview1 = "TRACEABILITY-1"

func init() {
	armerrors.Register(codeReview1)
}

func looksLikeJSONArg(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func readAssessmentFile(path string) ([]byte, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(filepath.Clean(path))
	}
	if err != nil {
		return nil, fmt.Errorf("read assessment file: %w", err)
	}
	return data, nil
}

func mapReviewError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	var skip protocolExitError
	if errors.As(err, &skip) {
		return err
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "--issue is required"),
		strings.Contains(msg, "--base is required"),
		strings.Contains(msg, "--head is required"),
		strings.Contains(msg, "--assessment is required"),
		strings.Contains(msg, "--bundle is required"),
		strings.Contains(msg, "issue ID is required"),
		strings.Contains(msg, "conflicting issue ID"),
		strings.Contains(msg, "accepts at most"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm review --help"}, err)
	case strings.Contains(msg, "read assessment file"),
		strings.Contains(msg, "parse assessment JSON"),
		strings.Contains(msg, "assessment validation failed"),
		strings.Contains(msg, "assessment coverage validation errors"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"jq empty <assessment.json>",
			"arm review record --assessment <assessment.json>",
		}, err)
	case strings.Contains(msg, "failed to list commits"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review commits --issue <issue-id> --branch <reachable-branch>",
		}, err)
	case strings.Contains(msg, "failed to resolve base revision"),
		strings.Contains(msg, "failed to resolve head revision"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review prepare --issue <issue-id> --base <reachable-ref> --head <reachable-ref>",
		}, err)
	case strings.Contains(msg, "not JSON content"),
		strings.Contains(msg, "read bundle file"):
		return armerrors.Wrap(codeReview1, msg, []string{
			"arm review prepare --output <bundle.json>",
			"arm review record --bundle <bundle.json>",
		}, err)
	case strings.Contains(msg, "issue") && strings.Contains(msg, "not found"):
		return armerrors.Wrap(codeReview1, msg, []string{"arm list", "arm show"}, err)
	case strings.Contains(msg, "load snapshot"):
		return armerrors.Wrap(codeReview1, msg, []string{"arm doctor"}, err)
	default:
		return armerrors.Wrap(codeReview1, msg, []string{"arm review prepare --output <bundle.json>"}, err)
	}
}
