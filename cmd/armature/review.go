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

	// Load snapshot to get issue metadata
	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
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

	// Validate structural correctness without diff-index citation checking.
	// When --bundle is provided, full diff-index citation validation is performed below.
	if errs := review.ValidateResultNoDiff(&assessment); len(errs) > 0 {
		msg := "assessment validation errors:"
		for _, e := range errs {
			msg += "\n  - " + e
		}
		return fmt.Errorf("%s", msg)
	}

	// Optionally load the review bundle for fingerprint verification and diff-index validation.
	var bundle review.ReviewBundle
	var bundleLoaded bool
	if bundleFile != "" {
		bundleData, err := os.ReadFile(filepath.Clean(bundleFile))
		if err != nil {
			return fmt.Errorf("read bundle file: %w", err)
		}
		if err := json.Unmarshal(bundleData, &bundle); err != nil {
			return fmt.Errorf("parse bundle JSON: %w", err)
		}
		bundleLoaded = true
	}

	// When the bundle is available, verify that the bundle was prepared for the correct issue.
	// This prevents assessment results from one issue (e.g., TASK-A) from being recorded onto
	// another issue (e.g., TASK-B) even if they share the same contract fingerprint.
	if bundleLoaded {
		if bundle.Issue.ID != issueID {
			return fmt.Errorf("bundle was prepared for issue %s, not %s",
				bundle.Issue.ID, issueID)
		}
	}

	// When the bundle is available, verify that the assessment's bundle_id and delivery_fingerprint
	// match the loaded bundle. This ensures the assessment was produced for this specific delivery
	// and prevents mismatched assessments from being recorded.
	if bundleLoaded {
		if assessment.BundleID != bundle.BundleID {
			return fmt.Errorf("assessment bundle_id %s does not match bundle bundle_id %s",
				assessment.BundleID, bundle.BundleID)
		}
		if assessment.DeliveryFingerprint != bundle.Fingerprints.Delivery {
			return fmt.Errorf("assessment delivery_fingerprint %s does not match bundle delivery_fingerprint %s",
				assessment.DeliveryFingerprint, bundle.Fingerprints.Delivery)
		}
		if assessment.ContractFingerprint != bundle.Fingerprints.Contract {
			return fmt.Errorf("assessment contract_fingerprint %s does not match bundle contract_fingerprint %s",
				assessment.ContractFingerprint, bundle.Fingerprints.Contract)
		}
	}

	// When the bundle is available, perform diff-index citation coordinate validation.
	// This ensures every citation references a line that actually appears in the delivery diff.
	if bundleLoaded {
		idx, err := review.BuildDiffIndex(bundle.Delivery.Diff)
		if err != nil {
			return fmt.Errorf("build diff index: %w", err)
		}
		if errs := review.ValidateResult(&assessment, idx); len(errs) > 0 {
			msg := "assessment citation validation errors:"
			for _, e := range errs {
				msg += "\n  - " + e
			}
			return fmt.Errorf("%s", msg)
		}
	}

	// Create attestation, populating BaseSHA/HeadSHA from the bundle delivery when available.
	var delivery review.Delivery
	if bundleLoaded {
		delivery = bundle.Delivery
	}
	att := review.NewAttestation(&assessment, delivery)

	// Load snapshot to check for duplicates
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir

	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	issuePtr, ok := snap.Issues[issueID]
	if !ok || issuePtr == nil {
		return fmt.Errorf("issue %q not found", issueID)
	}
	issue := *issuePtr

	// Build contract from issue for coverage validation and fingerprint check.
	criteria, err := review.ParseAcceptanceCriteria(issue.Acceptance)
	if err != nil {
		return fmt.Errorf("failed to parse acceptance criteria: %w", err)
	}
	contract := review.Contract{
		DefinitionOfDone: issue.DefinitionOfDone,
		Scope:            issue.Scope,
		Acceptance:       criteria,
	}

	// Verify the assessment's contract fingerprint matches the issue's contract.
	// A mismatch indicates the assessment was produced against a different (possibly stale
	// or fabricated) contract and must be rejected.
	issueContractFP := review.FingerprintContract(contract)
	if assessment.ContractFingerprint != issueContractFP {
		return fmt.Errorf("assessment contract fingerprint %s does not match issue contract fingerprint %s",
			assessment.ContractFingerprint, issueContractFP)
	}

	// Validate that assessment covers all expected criteria
	if errs := review.ValidateResultCoverage(&assessment, contract); len(errs) > 0 {
		msg := "assessment coverage validation errors:"
		for _, e := range errs {
			msg += "\n  - " + e
		}
		return fmt.Errorf("%s", msg)
	}

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
