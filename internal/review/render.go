package review

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// escapeMarkdownTableCell escapes special characters in Markdown table cells.
// Escapes pipes (| → \|), backticks (` → \`), and normalizes newlines to spaces.
func escapeMarkdownTableCell(s string) string {
	// First, normalize newlines to spaces
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")

	// Escape pipes
	s = strings.ReplaceAll(s, "|", "\\|")

	// Escape backticks
	s = strings.ReplaceAll(s, "`", "\\`")

	return s
}

// escapeMarkdownSpecialChars escapes pipes and backticks in all markdown contexts.
func escapeMarkdownSpecialChars(s string) string {
	// Escape pipes
	s = strings.ReplaceAll(s, "|", "\\|")

	// Escape backticks
	s = strings.ReplaceAll(s, "`", "\\`")

	return s
}

// RenderMarkdown renders a ConformanceAssessment as a safe Markdown report.
// Table cells are escaped to prevent pipes, backticks, and newlines from breaking Markdown syntax.
// All user-supplied strings are HTML-escaped.
func RenderMarkdown(a *ConformanceAssessment) string {
	var sb strings.Builder

	// Determine overall rating
	rating := DeriveRating(a.Results)

	// Write header
	sb.WriteString("# Conformance Assessment\n\n")
	fmt.Fprintf(&sb, "**Bundle ID:** %s\n\n", html.EscapeString(a.BundleID))

	// Write summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Criterion ID | Status | Rating |\n")
	sb.WriteString("|---|---|---|\n")

	for _, result := range a.Results {
		cellID := escapeMarkdownTableCell(html.EscapeString(result.ID))
		cellStatus := escapeMarkdownTableCell(html.EscapeString(result.Status.String()))
		cellRating := escapeMarkdownTableCell(html.EscapeString(rating.String()))
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", cellID, cellStatus, cellRating)
	}

	sb.WriteString("\n")

	// Write detailed results
	sb.WriteString("## Detailed Results\n\n")

	for _, result := range a.Results {
		escapedID := escapeMarkdownSpecialChars(html.EscapeString(result.ID))
		escapedStatus := escapeMarkdownSpecialChars(html.EscapeString(result.Status.String()))
		escapedRationale := escapeMarkdownSpecialChars(html.EscapeString(result.Rationale))

		fmt.Fprintf(&sb, "### %s\n\n", escapedID)
		fmt.Fprintf(&sb, "**Status:** %s\n\n", escapedStatus)
		fmt.Fprintf(&sb, "**Rationale:** %s\n\n", escapedRationale)

		if len(result.Citations) > 0 {
			sb.WriteString("**Citations:**\n\n")
			for _, citation := range result.Citations {
				// Activity citation: show entry details
				if citation.ActivityEntryID != "" {
					escapedDetails := escapeMarkdownSpecialChars(html.EscapeString(citation.ActivityEntryDetails))
					fmt.Fprintf(&sb, "- Activity: %s\n", escapedDetails)
				} else {
					// Diff citation: show file and line
					escapedPath := escapeMarkdownSpecialChars(html.EscapeString(citation.Path))
					if citation.Line > 0 {
						fmt.Fprintf(&sb, "- %s (line %d)\n",
							escapedPath, citation.Line)
					} else {
						fmt.Fprintf(&sb, "- %s\n",
							escapedPath)
					}
				}
			}
			sb.WriteString("\n")
		}

		if result.MissingEvidence != "" {
			escapedMissing := escapeMarkdownSpecialChars(html.EscapeString(result.MissingEvidence))
			fmt.Fprintf(&sb, "**Missing Evidence:** %s\n\n",
				escapedMissing)
		}
	}

	return sb.String()
}

// RenderHuman renders a ConformanceAssessment as plain human-readable text.
func RenderHuman(a *ConformanceAssessment) string {
	var sb strings.Builder

	// Determine overall rating and counts
	rating := DeriveRating(a.Results)
	satisfied, partiallySatisfied, notSatisfied, indeterminate := CountCriteria(a.Results)

	// Write header
	sb.WriteString("Conformance Assessment\n")
	sb.WriteString("=======================\n\n")

	fmt.Fprintf(&sb, "Bundle ID: %s\n", a.BundleID)
	fmt.Fprintf(&sb, "Contract Fingerprint: %s\n", a.ContractFingerprint)
	fmt.Fprintf(&sb, "Delivery Fingerprint: %s\n", a.DeliveryFingerprint)
	fmt.Fprintf(&sb, "Overall Rating: %s\n\n", rating.String())

	// Write summary counts
	sb.WriteString("Summary:\n")
	fmt.Fprintf(&sb, "  Satisfied: %d\n", satisfied)
	fmt.Fprintf(&sb, "  Partially Satisfied: %d\n", partiallySatisfied)
	fmt.Fprintf(&sb, "  Not Satisfied: %d\n", notSatisfied)
	fmt.Fprintf(&sb, "  Indeterminate: %d\n\n", indeterminate)

	// Write detailed results
	sb.WriteString("Results:\n")
	sb.WriteString("--------\n\n")

	for _, result := range a.Results {
		fmt.Fprintf(&sb, "Criterion: %s\n", result.ID)
		fmt.Fprintf(&sb, "  Status: %s\n", result.Status.String())
		fmt.Fprintf(&sb, "  Rationale: %s\n", result.Rationale)

		if len(result.Citations) > 0 {
			sb.WriteString("  Citations:\n")
			for _, citation := range result.Citations {
				// Activity citation: show entry details
				if citation.ActivityEntryID != "" {
					fmt.Fprintf(&sb, "    - Activity: %s\n", citation.ActivityEntryDetails)
				} else {
					// Diff citation: show file and line
					if citation.Line > 0 {
						fmt.Fprintf(&sb, "    - %s (line %d)\n", citation.Path, citation.Line)
					} else {
						fmt.Fprintf(&sb, "    - %s\n", citation.Path)
					}
				}
			}
		}

		if result.MissingEvidence != "" {
			fmt.Fprintf(&sb, "  Missing Evidence: %s\n", result.MissingEvidence)
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderJSON renders a ConformanceAssessment as indented JSON.
func RenderJSON(a *ConformanceAssessment) (string, error) {
	jsonBytes, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
