package output

import (
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	// ShowLargeFieldLimit is the default truncation cap for outcome and
	// definition_of_done in structured arm show output.
	ShowLargeFieldLimit = 512
	ShowFieldHelp       = "arm show <id> --field <name> extracts a scalar value, never an envelope"
)

// ShowTruncation is the truncated adjunct: large text fields stay present,
// with shown and total byte counts so agents can request --full.
type ShowTruncation struct {
	Field      string `json:"field"`
	ShownBytes int    `json:"shown_bytes"`
	TotalBytes int    `json:"total_bytes"`
}

// TruncateShowText returns a UTF-8-safe prefix of s no longer than limit.
func TruncateShowText(s string, limit int) (shown string, total int, truncated bool) {
	total = len(s)
	if total <= limit {
		return s, total, false
	}
	shown = s
	for len(shown) > limit {
		_, size := utf8.DecodeLastRuneInString(shown)
		if size <= 0 {
			break
		}
		shown = shown[:len(shown)-size]
	}
	return shown, total, true
}

// TruncateShowIssue mutates row in place, capping outcome and definition_of_done.
func TruncateShowIssue(row *IssueJSON) []ShowTruncation {
	var hints []ShowTruncation
	if shown, total, truncated := TruncateShowText(row.Outcome, ShowLargeFieldLimit); truncated {
		row.Outcome = shown
		hints = append(hints, ShowTruncation{Field: "outcome", ShownBytes: len(shown), TotalBytes: total})
	}
	if shown, total, truncated := TruncateShowText(row.DefinitionOfDone, ShowLargeFieldLimit); truncated {
		row.DefinitionOfDone = shown
		hints = append(hints, ShowTruncation{
			Field:      "definition_of_done",
			ShownBytes: len(shown),
			TotalBytes: total,
		})
	}
	return hints
}

// ShowHelp is the trailing help for a show envelope.
func ShowHelp(ids []string, trunc []ShowTruncation) []string {
	if len(trunc) == 0 {
		return []string{ShowFieldHelp}
	}
	id := "<id>"
	if len(ids) == 1 {
		id = ids[0]
	}
	t := trunc[0]
	line := fmt.Sprintf("%s truncated (%d of %d bytes); arm show %s --full for complete fields",
		t.Field, t.ShownBytes, t.TotalBytes, id)
	return []string{line, ShowFieldHelp}
}

// WriteShowEnvelope emits the compact agent show object {count,issues,help}
// and optional truncated adjunct. This is the live arm show json/agent path.
func WriteShowEnvelope(w io.Writer, ids []string, rows []IssueJSON, trunc []ShowTruncation) error {
	env, err := NewEnvelope("issues", rows, ShowHelp(ids, trunc))
	if err != nil {
		return err
	}
	if len(trunc) > 0 {
		if err := env.AddAdjunct("truncated", trunc); err != nil {
			return err
		}
	}
	return WriteEnvelope(w, env)
}
