package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/review"
	"github.com/spf13/cobra"
)

type reviewValidateFailure struct {
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	Fixable    bool   `json:"fixable"`
}

type reviewValidateReport struct {
	Valid    bool                    `json:"valid"`
	Failures []reviewValidateFailure `json:"failures,omitempty"`
}

func newReviewValidateCmd() *cobra.Command {
	var assessmentFile, bundleFile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a conformance assessment without recording it",
		Long: `Validate a conformance assessment against a review bundle using the same
checks arm review record performs (schema, criterion-ID format, citation
line-bounds, coverage, activity evidence). Failures include auto-fix
suggestions. No ops are appended; record remains the enforcement gate.`,
		Args: func(cmd *cobra.Command, args []string) error {
			return mapReviewError(cobra.MaximumNArgs(0)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapReviewError(runReviewValidate(cmd, assessmentFile, bundleFile))
		},
	}

	cmd.Flags().StringVar(&assessmentFile, "assessment", "", "assessment file path or '-' for stdin (required)")
	cmd.Flags().StringVar(&bundleFile, "bundle", "", "review bundle file path (required)")

	return cmd
}

func runReviewValidate(cmd *cobra.Command, assessmentFile, bundleFile string) error {
	if assessmentFile == "" {
		return fmt.Errorf("--assessment is required")
	}
	if bundleFile == "" {
		return fmt.Errorf("--bundle is required")
	}
	if looksLikeJSONArg(bundleFile) {
		return fmt.Errorf("--bundle expects a file path, not JSON content")
	}

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
		return emitReviewValidateResult(cmd, review.AnnotateValidateError(fmt.Errorf("parse assessment JSON: %w", err)))
	}

	bundleData, err := os.ReadFile(filepath.Clean(bundleFile))
	if err != nil {
		return fmt.Errorf("read bundle file: %w", err)
	}
	bundle, err := review.DecodeReviewBundle(bundleData)
	if err != nil {
		return emitReviewValidateResult(cmd, review.AnnotateValidateError(fmt.Errorf("parse bundle JSON: %w", err)))
	}
	if err := bundle.Valid(); err != nil {
		return emitReviewValidateResult(cmd, review.AnnotateValidateError(err))
	}

	issueID := bundle.Issue.ID
	if issueID == "" {
		return fmt.Errorf("bundle is missing issue ID")
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

	input := review.RecordInput{
		Assessment: &assessment,
		Bundle:     &bundle,
		Issue: &review.IssueData{
			DefinitionOfDone: issue.DefinitionOfDone,
			Scope:            issue.Scope,
			Acceptance:       string(issue.Acceptance),
		},
		IssueID: issueID,
	}

	return emitReviewValidateResult(cmd, review.ValidateAssessment(input))
}

func emitReviewValidateResult(cmd *cobra.Command, validateErr error) error {
	report := reviewValidateReport{Valid: validateErr == nil}
	if validateErr != nil {
		report.Failures = parseReviewValidateFailures(validateErr)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if err := writeReviewValidateReport(cmd, format, report); err != nil {
		return err
	}
	if !report.Valid {
		return skipCommandFailure(validateErr)
	}
	return nil
}

func writeReviewValidateReport(cmd *cobra.Command, format string, report reviewValidateReport) error {
	out := cmd.OutOrStdout()
	if format == "json" || format == "agent" {
		data, err := json.Marshal(report)
		if err != nil {
			return fmt.Errorf("marshal validation report: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(data))
		return nil
	}

	if report.Valid {
		_, _ = fmt.Fprintln(out, "Assessment is valid")
		return nil
	}
	_, _ = fmt.Fprintln(out, "Assessment is invalid:")
	for _, failure := range report.Failures {
		_, _ = fmt.Fprintf(out, "  - %s\n", failure.Message)
		if failure.Suggestion != "" {
			_, _ = fmt.Fprintf(out, "    suggestion: %s\n", failure.Suggestion)
		}
	}
	return nil
}

func parseReviewValidateFailures(err error) []reviewValidateFailure {
	if err == nil {
		return nil
	}
	var failures []reviewValidateFailure
	for _, line := range strings.Split(err.Error(), "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "- ")
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, ":") {
			continue
		}
		msg, suggestion := splitReviewValidateSuggestion(trimmed)
		fix := review.ClassifyValidateFix(msg)
		if suggestion == "" {
			suggestion = fix.Suggestion
		}
		failures = append(failures, reviewValidateFailure{Message: msg, Suggestion: suggestion, Fixable: fix.Fixable})
	}
	if len(failures) == 0 {
		msg, suggestion := splitReviewValidateSuggestion(err.Error())
		fix := review.ClassifyValidateFix(msg)
		if suggestion == "" {
			suggestion = fix.Suggestion
		}
		failures = append(failures, reviewValidateFailure{Message: msg, Suggestion: suggestion, Fixable: fix.Fixable})
	}
	return failures
}

func splitReviewValidateSuggestion(line string) (string, string) {
	const marker = " (suggestion: "
	i := strings.LastIndex(line, marker)
	if i < 0 || !strings.HasSuffix(line, ")") {
		return line, ""
	}
	return line[:i], strings.TrimSuffix(line[i+len(marker):], ")")
}
