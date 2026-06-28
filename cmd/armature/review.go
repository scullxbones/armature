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
	"github.com/scullxbones/armature/internal/snapshot"
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
	issuesDir := ctx.IssuesDir
	singleBranch := ctx.Mode == "single-branch"

	// Load snapshot to get issue metadata
	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
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
	var criteria []string
	if issue.Acceptance != nil {
		if err := json.Unmarshal(issue.Acceptance, &criteria); err != nil {
			return fmt.Errorf("failed to parse acceptance criteria: %w", err)
		}
	}

	// Create git adapter
	git := adapters.New(ctx.RepoPath)

	// Call prepare
	bundle, err := review.Prepare(git, issueID, title, scope, criteria, base, head)
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
	var issueID, assessmentFile string

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a conformance assessment for an issue",
		Long: `Record a conformance assessment for an issue by reading a ConformanceAssessment
from a JSON file and writing an assessment-attested operation.

The assessment file should contain a valid ConformanceAssessment JSON object.
If the assessment is a duplicate of an existing one (same bundle ID), the command
is idempotent and returns success without writing a duplicate operation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewRecord(cmd, issueID, assessmentFile)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID (required)")
	cmd.Flags().StringVar(&assessmentFile, "assessment", "", "assessment file path or '-' for stdin (required)")

	return cmd
}

func runReviewRecord(cmd *cobra.Command, issueID, assessmentFile string) error {
	if issueID == "" {
		return fmt.Errorf("--issue is required")
	}
	if assessmentFile == "" {
		return fmt.Errorf("--assessment is required")
	}

	// Read assessment JSON
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

	// Parse assessment
	var assessment review.ConformanceAssessment
	if err := json.Unmarshal(assessmentData, &assessment); err != nil {
		return fmt.Errorf("parse assessment JSON: %w", err)
	}

	// Validate the assessment structure
	if err := assessment.Valid(); err != nil {
		return fmt.Errorf("assessment validation failed: %w", err)
	}

	// Build DiffIndex for citation validation (empty for now since we don't have the diff)
	// The validation will only check that non-satisfied criteria have missing evidence if no citations
	idx, err := review.BuildDiffIndex("")
	if err != nil {
		return fmt.Errorf("build diff index: %w", err)
	}

	// Validate citations against the diff index
	if errs := review.ValidateResult(&assessment, idx); len(errs) > 0 {
		msg := "assessment validation errors:"
		for _, e := range errs {
			msg += "\n  - " + e
		}
		return fmt.Errorf("%s", msg)
	}

	// Create attestation
	att := review.NewAttestation(&assessment)

	// Load snapshot to check for duplicates
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir
	singleBranch := ctx.Mode == "single-branch"

	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	issuePtr, ok := snap.Issues[issueID]
	if !ok || issuePtr == nil {
		return fmt.Errorf("issue %q not found", issueID)
	}
	issue := *issuePtr

	// Check for duplicate attestations
	for _, existingAtt := range issue.AssessmentAttestations {
		if review.IsDuplicate(att, &existingAtt) {
			// Idempotent: duplicate is acceptable
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "status": "duplicate", "bundle_id": att.BundleID}
				data, _ := json.Marshal(result) //nolint:errcheck
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s already recorded (idempotent)\n", att.BundleID)
			}
			return nil
		}
	}

	// Marshal attestation as JSON RawMessage
	attJSON, err := json.Marshal(att)
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}

	// Create and write operation
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
		result := map[string]string{
			"issue":     issueID,
			"status":    "recorded",
			"bundle_id": att.BundleID,
			"rating":    att.Rating.String(),
		}
		data, _ := json.Marshal(result) //nolint:errcheck
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assessment for bundle %s recorded with rating %s\n", att.BundleID, att.Rating.String())
	}

	return nil
}
