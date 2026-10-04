package output

import "io"

// SyncIssue is one done-leaf row in the arm sync ADR 0017 envelope.
type SyncIssue struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	NextAction string `json:"next_action,omitempty"`
}

const (
	syncHelpEmpty = "arm list --status done"
)

var syncKindRank = map[string]int{
	"missing-assessment": 0,
	"check-failed":       1,
	"legacy":             2,
	"empty-diff":         3,
	"not-on-target":      4,
	"not-leaf":           5,
	"promote":            6,
}

// SyncHelp returns help[0] as the most actionable arm command for rows.
func SyncHelp(rows []SyncIssue) []string {
	if len(rows) == 0 {
		return []string{syncHelpEmpty}
	}
	best := rows[0]
	bestRank := syncKindRankOrLast(best.Kind)
	for _, row := range rows[1:] {
		if r := syncKindRankOrLast(row.Kind); r < bestRank {
			best, bestRank = row, r
		}
	}
	if best.NextAction != "" {
		return []string{best.NextAction}
	}
	return []string{"arm show " + best.ID}
}

func syncKindRankOrLast(kind string) int {
	if r, ok := syncKindRank[kind]; ok {
		return r
	}
	return len(syncKindRank)
}

// WriteSyncEnvelope emits {count,issues,help} for arm sync and the post-merge hook.
func WriteSyncEnvelope(w io.Writer, rows []SyncIssue) error {
	env, err := NewEnvelope("issues", rows, SyncHelp(rows))
	if err != nil {
		return err
	}
	return WriteEnvelope(w, env)
}
