package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/validate"
)

// ListEntry represents a row in a list view of issues.
// Only fields rendered by RenderList are included; add fields here only when they
// are both populated by callers and rendered in output.
type ListEntry struct {
	Issue      string `json:"issue"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	AssignedTo string `json:"assigned_to,omitempty"`
}

// RenderIssue renders a single issue to the given writer.
// If asJSON is true, renders as JSON; otherwise renders as human-readable text.
func RenderIssue(w io.Writer, issue *materialize.Issue, asJSON bool) error {
	if asJSON {
		return renderIssueJSON(w, issue)
	}
	return renderIssueHuman(w, issue)
}

// IssueJSON is the canonical JSON representation of an issue.
// All JSON serialization of issues must go through this struct to ensure schema consistency.
type IssueJSON struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	Type             string          `json:"type"`
	Status           string          `json:"status"`
	Parent           string          `json:"parent,omitempty"`
	Priority         string          `json:"priority,omitempty"`
	DefinitionOfDone string          `json:"definition_of_done,omitempty"`
	Scope            []string        `json:"scope,omitempty"`
	Outcome          string          `json:"outcome,omitempty"`
	ClaimedBy        string          `json:"claimed_by,omitempty"`
	AssignedWorker   string          `json:"assigned_worker,omitempty"`
	BlockedBy        []string        `json:"blocked_by,omitempty"`
	Blocks           []string        `json:"blocks,omitempty"`
	Acceptance       json.RawMessage `json:"acceptance,omitempty"`
	Notes            []string        `json:"notes,omitempty"`
}

// MarshalIssue converts a materialize.Issue to the canonical IssueJSON representation.
func MarshalIssue(issue *materialize.Issue) IssueJSON {
	noteTexts := make([]string, 0, len(issue.Notes))
	for _, n := range issue.Notes {
		if !n.Deleted {
			noteTexts = append(noteTexts, n.Msg)
		}
	}
	return IssueJSON{
		ID:               issue.ID,
		Title:            issue.Title,
		Type:             issue.Type,
		Status:           issue.Status,
		Parent:           issue.Parent,
		Priority:         issue.Priority,
		DefinitionOfDone: issue.DefinitionOfDone,
		Scope:            issue.Scope,
		Outcome:          issue.Outcome,
		ClaimedBy:        issue.ClaimedBy,
		AssignedWorker:   issue.AssignedWorker,
		BlockedBy:        issue.BlockedBy,
		Blocks:           issue.Blocks,
		Acceptance:       issue.Acceptance,
		Notes:            noteTexts,
	}
}

func renderIssueJSON(w io.Writer, issue *materialize.Issue) error {
	out := MarshalIssue(issue)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue JSON: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func renderIssueHuman(w io.Writer, issue *materialize.Issue) error {
	_, err := fmt.Fprintf(w, "ID:        %s\n", issue.ID)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Title:     %s\n", issue.Title)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Type:      %s\n", issue.Type)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Status:    %s\n", issue.Status)
	if err != nil {
		return err
	}

	if issue.Parent != "" {
		_, err = fmt.Fprintf(w, "Parent:    %s\n", issue.Parent)
		if err != nil {
			return err
		}
	}

	if issue.Priority != "" {
		_, err = fmt.Fprintf(w, "Priority:  %s\n", issue.Priority)
		if err != nil {
			return err
		}
	}

	if issue.ClaimedBy != "" {
		_, err = fmt.Fprintf(w, "ClaimedBy: %s\n", issue.ClaimedBy)
		if err != nil {
			return err
		}
	}

	if issue.AssignedWorker != "" {
		_, err = fmt.Fprintf(w, "Assigned:  %s\n", issue.AssignedWorker)
		if err != nil {
			return err
		}
	}

	if issue.DefinitionOfDone != "" {
		_, err = fmt.Fprintf(w, "DoD:       %s\n", issue.DefinitionOfDone)
		if err != nil {
			return err
		}
	}

	if len(issue.Acceptance) > 0 && string(issue.Acceptance) != "null" {
		compact, err := json.Marshal(issue.Acceptance)
		if err != nil {
			return fmt.Errorf("marshal acceptance: %w", err)
		}
		_, err = fmt.Fprintf(w, "Acceptance: %s\n", string(compact))
		if err != nil {
			return err
		}
	}

	if len(issue.Scope) > 0 {
		_, err = fmt.Fprintf(w, "Scope:     %s\n", strings.Join(issue.Scope, ", "))
		if err != nil {
			return err
		}
	}

	if issue.Outcome != "" {
		_, err = fmt.Fprintf(w, "Outcome:   %s\n", issue.Outcome)
		if err != nil {
			return err
		}
	}

	if len(issue.BlockedBy) > 0 {
		_, err = fmt.Fprintf(w, "BlockedBy: %s\n", strings.Join(issue.BlockedBy, ", "))
		if err != nil {
			return err
		}
	}

	if len(issue.Blocks) > 0 {
		_, err = fmt.Fprintf(w, "Blocks:    %s\n", strings.Join(issue.Blocks, ", "))
		if err != nil {
			return err
		}
	}

	activeNotes := make([]materialize.Note, 0, len(issue.Notes))
	for _, n := range issue.Notes {
		if !n.Deleted {
			activeNotes = append(activeNotes, n)
		}
	}
	if len(activeNotes) > 0 {
		_, err = fmt.Fprintf(w, "Notes:\n")
		if err != nil {
			return err
		}
		for _, n := range activeNotes {
			_, err = fmt.Fprintf(w, "  - %s\n", n.Msg)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// RenderList renders a list of issues to the given writer.
// Each entry is rendered as a single line in human-readable format.
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

// RenderReady renders the ready queue to the given writer.
// If asJSON is true, renders as JSON; otherwise renders as human-readable text.
func RenderReady(w io.Writer, entries []ready.ReadyEntry, asJSON bool) error {
	if asJSON {
		return renderReadyJSON(w, entries)
	}
	return renderReadyHuman(w, entries)
}

func renderReadyJSON(w io.Writer, entries []ready.ReadyEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ready queue JSON: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func renderReadyHuman(w io.Writer, entries []ready.ReadyEntry) error {
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

// RenderValidation renders validation results to the given writer.
// Displays errors, warnings, infos, coverage, and OK status using prefixed lines.
// When quiet is true, INFO lines are suppressed; COVERAGE and OK lines are always shown.
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
	if result.Coverage != nil {
		cov := result.Coverage
		totalCited := cov.CitedNodes + cov.AcceptedRiskNodes
		if cov.AcceptedRiskNodes > 0 {
			if _, err := fmt.Fprintf(w, "COVERAGE: %d/%d cited (%d source-linked, %d accepted-risk)\n",
				totalCited, cov.TotalNodes, cov.CitedNodes, cov.AcceptedRiskNodes); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "COVERAGE: %d/%d cited\n", totalCited, cov.TotalNodes); err != nil {
				return err
			}
		}
	}
	if result.OK {
		if _, err := fmt.Fprintln(w, "OK: no issues found"); err != nil {
			return err
		}
	}
	return nil
}
