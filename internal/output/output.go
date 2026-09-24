// Package output formats command results for human and machine (JSON) consumption across the CLI.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/review"
	"github.com/scullxbones/armature/internal/validate"
)

type ListEntry struct {
	Issue      string `json:"issue"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	AssignedTo string `json:"assigned_to,omitempty"`
}

type IssueJSON struct {
	ID                     string          `json:"id"`
	Title                  string          `json:"title"`
	Type                   string          `json:"type"`
	Status                 string          `json:"status"`
	Parent                 string          `json:"parent,omitempty"`
	Priority               string          `json:"priority,omitempty"`
	DefinitionOfDone       string          `json:"definition_of_done,omitempty"`
	Scope                  []string        `json:"scope,omitempty"`
	Outcome                string          `json:"outcome,omitempty"`
	ClaimedBy              string          `json:"claimed_by,omitempty"`
	AssignedWorker         string          `json:"assigned_worker,omitempty"`
	BlockedBy              []string        `json:"blocked_by,omitempty"`
	Blocks                 []string        `json:"blocks,omitempty"`
	Acceptance             json.RawMessage `json:"acceptance,omitempty"`
	Notes                  []string        `json:"notes,omitempty"`
	AssessmentAttestations json.RawMessage `json:"assessment_attestations,omitempty"`
}

func MarshalIssue(issue *materialize.Issue) IssueJSON {
	noteTexts := make([]string, 0, len(issue.Notes))
	for _, n := range issue.Notes {
		if !n.Deleted {
			noteTexts = append(noteTexts, n.Msg)
		}
	}

	var attestationsJSON json.RawMessage
	if len(issue.AssessmentAttestations) > 0 {
		data, err := json.Marshal(issue.AssessmentAttestations)
		if err == nil {
			attestationsJSON = json.RawMessage(data)
		}
	}

	return IssueJSON{
		ID:                     issue.ID,
		Title:                  issue.Title,
		Type:                   issue.Type,
		Status:                 issue.Status,
		Parent:                 issue.Parent,
		Priority:               issue.Priority,
		DefinitionOfDone:       issue.DefinitionOfDone,
		Scope:                  issue.Scope,
		Outcome:                issue.Outcome,
		ClaimedBy:              issue.ClaimedBy,
		AssignedWorker:         issue.AssignedWorker,
		BlockedBy:              issue.BlockedBy,
		Blocks:                 issue.Blocks,
		Acceptance:             issue.Acceptance,
		Notes:                  noteTexts,
		AssessmentAttestations: attestationsJSON,
	}
}

type firstErrorWriter struct {
	w   io.Writer
	err error
}

func (ew *firstErrorWriter) printf(format string, args ...interface{}) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}

const bundleIDDisplayHexLen = 12

func truncateBundleID(bundleID string) string {
	id := strings.TrimPrefix(bundleID, "sha256:")
	if len(id) <= bundleIDDisplayHexLen {
		return id
	}
	return id[:bundleIDDisplayHexLen]
}

func formatLatestAttestationLine(attestations []review.AssessmentAttestation) string {
	if len(attestations) == 0 {
		return ""
	}

	att := attestations[len(attestations)-1]
	bundleID := truncateBundleID(att.BundleID)
	ratingStr := att.Rating.String()

	var counts []string
	if att.SatisfiedCount > 0 {
		counts = append(counts, fmt.Sprintf("%d satisfied", att.SatisfiedCount))
	}
	if att.PartiallySatisfiedCount > 0 {
		counts = append(counts, fmt.Sprintf("%d partially_satisfied", att.PartiallySatisfiedCount))
	}
	if att.NotSatisfiedCount > 0 {
		counts = append(counts, fmt.Sprintf("%d not_satisfied", att.NotSatisfiedCount))
	}
	if att.IndeterminateCount > 0 {
		counts = append(counts, fmt.Sprintf("%d indeterminate", att.IndeterminateCount))
	}

	if len(counts) == 0 {
		return fmt.Sprintf("Review:    %s (bundle %s)", ratingStr, bundleID)
	}

	return fmt.Sprintf("Review:    %s (bundle %s; %s)", ratingStr, bundleID, strings.Join(counts, ", "))
}

func RenderIssue(w io.Writer, issue *materialize.Issue) error {
	ew := &firstErrorWriter{w: w}

	ew.printf("ID:        %s\n", issue.ID)
	ew.printf("Title:     %s\n", issue.Title)
	ew.printf("Type:      %s\n", issue.Type)
	ew.printf("Status:    %s\n", issue.Status)

	if line := formatLatestAttestationLine(issue.AssessmentAttestations); line != "" {
		ew.printf("%s\n", line)
	}

	if issue.Parent != "" {
		ew.printf("Parent:    %s\n", issue.Parent)
	}
	if issue.Priority != "" {
		ew.printf("Priority:  %s\n", issue.Priority)
	}
	if issue.ClaimedBy != "" {
		ew.printf("ClaimedBy: %s\n", issue.ClaimedBy)
	}
	if issue.AssignedWorker != "" {
		ew.printf("Assigned:  %s\n", issue.AssignedWorker)
	}
	if issue.DefinitionOfDone != "" {
		ew.printf("DoD:       %s\n", issue.DefinitionOfDone)
	}

	if len(issue.Acceptance) > 0 && string(issue.Acceptance) != "null" {
		compact, err := json.Marshal(issue.Acceptance)
		if err != nil {
			return fmt.Errorf("marshal acceptance: %w", err)
		}
		ew.printf("Acceptance: %s\n", string(compact))
	}

	if len(issue.Scope) > 0 {
		ew.printf("Scope:     %s\n", strings.Join(issue.Scope, ", "))
	}
	if issue.Outcome != "" {
		ew.printf("Outcome:   %s\n", issue.Outcome)
	}
	if len(issue.BlockedBy) > 0 {
		ew.printf("BlockedBy: %s\n", strings.Join(issue.BlockedBy, ", "))
	}
	if len(issue.Blocks) > 0 {
		ew.printf("Blocks:    %s\n", strings.Join(issue.Blocks, ", "))
	}

	if ew.err != nil {
		return ew.err
	}

	activeNotes := make([]materialize.Note, 0, len(issue.Notes))
	for _, n := range issue.Notes {
		if !n.Deleted {
			activeNotes = append(activeNotes, n)
		}
	}
	if len(activeNotes) > 0 {
		ew.printf("Notes:\n")
		for _, n := range activeNotes {
			ew.printf("  - %s\n", n.Msg)
		}
	}

	return ew.err
}

func RenderList(w io.Writer, entries []ListEntry) error {
	if len(entries) == 0 {
		return nil
	}

	for _, e := range entries {
		status := e.Status
		if e.AssignedTo != "" {
			status = status + " (assigned to " + e.AssignedTo + ")"
		}
		_, err := fmt.Fprintf(w, "  %-12s  %-14s  %s\n", e.Issue, status, e.Title)
		if err != nil {
			return err
		}
	}
	return nil
}

type BoardEntry struct {
	Issue   string
	Status  string
	Claimed string
	Outcome string
	Title   string
}

func RenderBoard(w io.Writer, entries []BoardEntry) error {
	if len(entries) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "%-12s %-12s %-38s %-30s %s\n", "ID", "STATUS", "CLAIMED", "OUTCOME", "TITLE")
	if err != nil {
		return err
	}
	for _, e := range entries {
		outcome := e.Outcome
		const maxOutcome = 30
		if len(outcome) > maxOutcome {
			outcome = outcome[:27] + "..."
		}
		_, err = fmt.Fprintf(w, "%-12s %-12s %-38s %-30s %s\n", e.Issue, e.Status, e.Claimed, outcome, e.Title)
		if err != nil {
			return err
		}
	}
	return nil
}

func RenderExpiredClaims(w io.Writer, claims []ready.ExpiredClaimEntry) error {
	if len(claims) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Expired claims (TTL lapsed without renewal):"); err != nil {
		return err
	}
	for _, c := range claims {
		_, err := fmt.Fprintf(w, "  %s  %s  (%s, claimed by %s)\n", c.Issue, c.Title, c.Status, c.ClaimedBy)
		if err != nil {
			return err
		}
	}
	return nil
}

func RenderReady(w io.Writer, entries []ready.ReadyEntry) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "No tasks ready.")
		return err
	}

	for _, e := range entries {
		conf := ""
		if e.RequiresConfirmation {
			conf = " [requires confirmation]"
		}
		_, err := fmt.Fprintf(w, "  %s  %s  (%s)%s\n", e.Issue, e.Title, e.Priority, conf)
		if err != nil {
			return err
		}
	}
	return nil
}

func RenderValidation(w io.Writer, result validate.Result, quiet bool) error {
	for _, e := range result.Errors {
		if _, err := fmt.Fprintf(w, "ERROR: %s\n", e); err != nil {
			return err
		}
	}
	for _, warn := range result.Warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warn); err != nil {
			return err
		}
	}
	if !quiet {
		for _, info := range result.Infos {
			if _, err := fmt.Fprintf(w, "INFO: %s\n", info); err != nil {
				return err
			}
		}
	}
	if line := CoverageLine(result); line != "" {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if result.OK {
		if _, err := fmt.Fprintln(w, "OK: no issues found"); err != nil {
			return err
		}
	}
	return nil
}

func CoverageLine(result validate.Result) string {
	if result.Coverage == nil {
		return ""
	}
	cov := result.Coverage
	totalCited := cov.CitedNodes + cov.AcceptedRiskNodes
	if cov.AcceptedRiskNodes > 0 {
		return fmt.Sprintf("COVERAGE: %d/%d cited (%d source-linked, %d accepted-risk)",
			totalCited, cov.TotalNodes, cov.CitedNodes, cov.AcceptedRiskNodes)
	}
	return fmt.Sprintf("COVERAGE: %d/%d cited", totalCited, cov.TotalNodes)
}
