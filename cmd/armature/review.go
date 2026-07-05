package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
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
			return runReviewPrepare(cmd, issueID, base, head, outputFile)
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

	// Call prepare — pass real issue metadata (type, outcome, definition of done)
	bundle, err := review.Prepare(git, issueID, title, issue.DefinitionOfDone, issue.Type, issue.Outcome, scope, criteria, base, head)
	if err != nil {
		return fmt.Errorf("prepare review bundle: %w", err)
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
			return runReviewRecord(cmd, issueID, assessmentFile, bundleFile)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID (required)")
	cmd.Flags().StringVar(&assessmentFile, "assessment", "", "assessment file path or '-' for stdin (required)")
	cmd.Flags().StringVar(&bundleFile, "bundle", "", "review bundle file path (optional; enables diff-index citation validation)")

	return cmd
}

func runReviewRecord(cmd *cobra.Command, issueID, assessmentFile, bundleFile string) error {
	if issueID == "" {
		return fmt.Errorf("--issue is required")
	}
	if assessmentFile == "" {
		return fmt.Errorf("--assessment is required")
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

	var assessment review.ConformanceAssessment
	if err := json.Unmarshal(assessmentData, &assessment); err != nil {
		return fmt.Errorf("parse assessment JSON: %w", err)
	}

	// Optionally load the review bundle for fingerprint verification and diff-index validation.
	var bundlePtr *review.ReviewBundle
	if bundleFile != "" {
		bundleData, err := os.ReadFile(filepath.Clean(bundleFile))
		if err != nil {
			return fmt.Errorf("read bundle file: %w", err)
		}
		var bundle review.ReviewBundle
		if err := json.Unmarshal(bundleData, &bundle); err != nil {
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
