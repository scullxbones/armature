// Package output formats command results for human and machine (JSON) consumption across the CLI.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type HostRefJSON struct {
	URL     string `json:"url"`
	Role    string `json:"role,omitempty"`
	Current bool   `json:"current,omitempty"`
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
	HostRefs               []HostRefJSON   `json:"host_refs,omitempty"`
	AssessmentAttestations json.RawMessage `json:"assessment_attestations,omitempty"`
	Branch                 string          `json:"branch,omitempty"`
	Base                   string          `json:"base,omitempty"`
	Tip                    string          `json:"tip,omitempty"`
	PR                     string          `json:"pr,omitempty"`
	IntegrationBranch      string          `json:"integration_branch,omitempty"`
	TargetSHA              string          `json:"target_sha,omitempty"`
	CombinedPatchID        string          `json:"combined_patch_id,omitempty"`
	MatchedCommit          string          `json:"matched_commit,omitempty"`
	Derived                bool            `json:"derived,omitempty"`
}

func MarshalIssue(issue IssueFacts) IssueJSON {
	noteTexts := make([]string, 0, len(issue.Notes))
	for _, n := range issue.Notes {
		if !n.Deleted {
			noteTexts = append(noteTexts, n.Msg)
		}
	}
	hostRefs := make([]HostRefJSON, 0, len(issue.HostRefs))
	for _, ref := range issue.HostRefs {
		hostRefs = append(hostRefs, HostRefJSON(ref))
	}
	if len(hostRefs) == 0 {
		hostRefs = nil
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
		HostRefs:               hostRefs,
		AssessmentAttestations: issue.AssessmentAttestations,
		Branch:                 issue.Branch,
		Base:                   issue.Base,
		Tip:                    issue.Tip,
		PR:                     issue.PR,
		IntegrationBranch:      issue.IntegrationBranch,
		TargetSHA:              issue.TargetSHA,
		CombinedPatchID:        issue.CombinedPatchID,
		MatchedCommit:          issue.MatchedCommit,
		Derived:                issue.Derived,
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

type attestationView struct {
	BundleID                string `json:"bundle_id"`
	Rating                  string `json:"rating"`
	SatisfiedCount          int    `json:"satisfied_count"`
	PartiallySatisfiedCount int    `json:"partially_satisfied_count"`
	NotSatisfiedCount       int    `json:"not_satisfied_count"`
	IndeterminateCount      int    `json:"indeterminate_count"`
}

func formatLatestAttestationLine(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var attestations []attestationView
	if err := json.Unmarshal(raw, &attestations); err != nil || len(attestations) == 0 {
		return ""
	}

	att := attestations[len(attestations)-1]
	bundleID := truncateBundleID(att.BundleID)
	ratingStr := att.Rating

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

func RenderIssue(w io.Writer, issue IssueFacts) error {
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
	if issue.Branch != "" {
		ew.printf("Branch:    %s\n", issue.Branch)
	}
	if issue.Base != "" {
		ew.printf("Base:      %s\n", issue.Base)
	}
	if issue.Tip != "" {
		ew.printf("Tip:       %s\n", issue.Tip)
	}
	if issue.PR != "" {
		ew.printf("PR:        %s\n", issue.PR)
	}
	if issue.IntegrationBranch != "" {
		ew.printf("Integration: %s\n", issue.IntegrationBranch)
	}
	if issue.TargetSHA != "" {
		ew.printf("Target:    %s\n", issue.TargetSHA)
	}
	if issue.MatchedCommit != "" {
		ew.printf("Matched:   %s\n", issue.MatchedCommit)
	}
	if issue.Derived {
		ew.printf("Derived:   true\n")
	}
	if len(issue.BlockedBy) > 0 {
		ew.printf("BlockedBy: %s\n", strings.Join(issue.BlockedBy, ", "))
	}
	if len(issue.Blocks) > 0 {
		ew.printf("Blocks:    %s\n", strings.Join(issue.Blocks, ", "))
	}
	if len(issue.HostRefs) > 0 {
		ew.printf("HostRefs:\n")
		for _, ref := range issue.HostRefs {
			line := "  - " + ref.URL
			if ref.Role != "" {
				line += " role=" + ref.Role
			}
			if ref.Current {
				line += " current"
			}
			ew.printf("%s\n", line)
		}
	}

	if ew.err != nil {
		return ew.err
	}

	activeNotes := make([]NoteFact, 0, len(issue.Notes))
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

const (
	listOutcomeWidth = 30
	listDoneStatus   = "done"
)

// RenderList writes human arm list output. grouped prints status buckets and
// wins when both flags are set. parentBoard prints the --parent column table.
// Any other call prints the default three-column list. An empty row slice
// writes nothing, including no column header.
func RenderList(w io.Writer, rows []IssueFacts, grouped, parentBoard bool) error {
	if len(rows) == 0 {
		return nil
	}
	ew := &firstErrorWriter{w: w}
	switch {
	case grouped:
		renderGroupedList(ew, rows)
	case parentBoard:
		renderParentBoard(ew, rows)
	default:
		renderPlainList(ew, rows)
	}
	return ew.err
}

func renderPlainList(ew *firstErrorWriter, rows []IssueFacts) {
	for _, row := range rows {
		status := row.Status
		if row.AssignedWorker != "" {
			status = status + " (assigned to " + row.AssignedWorker + ")"
		}
		ew.printf("  %-12s  %-14s  %s\n", row.ID, status, row.Title)
		writeListHostRefs(ew, row.HostRefs)
	}
}

func renderParentBoard(ew *firstErrorWriter, rows []IssueFacts) {
	ew.printf("%-12s %-12s %-38s %-30s %s\n", "ID", "STATUS", "CLAIMED", "OUTCOME", "TITLE")
	for _, row := range rows {
		ew.printf("%-12s %-12s %-38s %-30s %s\n", row.ID, row.Status, row.ClaimedBy, truncateListOutcome(row.Outcome), row.Title)
		writeListHostRefs(ew, row.HostRefs)
	}
}

func renderGroupedList(ew *firstErrorWriter, rows []IssueFacts) {
	byID := make(map[string]IssueFacts, len(rows))
	facts := make(map[string]ListFacts, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		byID[row.ID] = row
		facts[row.ID] = ListFacts{Status: row.Status, Title: row.Title}
	}
	for _, g := range ListGroupsByStatus(facts, ids) {
		label := g.Status
		if g.Status == listDoneStatus {
			label = "done (awaiting merge)"
		}
		ew.printf("\n=== %s ===\n", label)
		for _, id := range g.IDs {
			row := byID[id]
			line := fmt.Sprintf("  %-12s  %s", id, row.Title)
			if g.Status == listDoneStatus && row.Branch != "" {
				line += fmt.Sprintf("  [branch: %s", row.Branch)
				if row.PR != "" {
					line += fmt.Sprintf(", PR: #%s", row.PR)
				}
				line += "]"
			}
			ew.printf("%s\n", line)
			writeListHostRefs(ew, row.HostRefs)
		}
	}
}

func truncateListOutcome(outcome string) string {
	if len(outcome) <= listOutcomeWidth {
		return outcome
	}
	return outcome[:listOutcomeWidth-3] + "..."
}

func writeListHostRefs(ew *firstErrorWriter, refs []HostRefFact) {
	for _, ref := range refs {
		line := ref.URL
		if ref.Role != "" {
			line += " role=" + ref.Role
		}
		if ref.Current {
			line += " current"
		}
		ew.printf("    %s\n", line)
	}
}

func RenderExpiredClaims(w io.Writer, claims []ExpiredClaim) error {
	if len(claims) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Expired claims (TTL lapsed without renewal):"); err != nil {
		return err
	}
	for _, c := range claims {
		_, err := fmt.Fprintf(w, "  %s  %s  (%s, claimed by %s)\n", c.ID, c.Title, c.Status, c.ClaimedBy)
		if err != nil {
			return err
		}
	}
	return nil
}

func RenderReady(w io.Writer, entries []ReadyIssue) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "No tasks ready.")
		return err
	}

	for _, e := range entries {
		conf := ""
		if e.RequiresConfirmation {
			conf = " [requires confirmation]"
		}
		_, err := fmt.Fprintf(w, "  %s  %s  (%s)%s\n", e.ID, e.Title, e.Priority, conf)
		if err != nil {
			return err
		}
	}
	return nil
}
