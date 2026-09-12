package output

import (
	"fmt"
	"io"

	"github.com/scullxbones/armature/internal/ready"
)

const (
	ReadyShowHelp         = "arm show <id> for outcome, scope, and acceptance"
	ReadyClaimHelp        = "arm claim --issue <id> --worktree"
	ReadyEmptyHelp        = "no issues are ready to claim; blockers are unmerged or claims are active"
	ReadyEmptyExpiredHelp = "no issues are ready to claim; expired_claims lists TTL-lapsed claims that are not in the queue"
	ReadyWavesHelp        = "dispatch one wave at a time"
	ReadyExpiredHelp      = "expired_claims lists TTL-lapsed claims; they are not in the ready queue"
)

// ReadyIssue is the structured ready-queue row: N4 keys plus ready-specific fields.
type ReadyIssue struct {
	ID                   string   `json:"id"`
	Type                 string   `json:"type"`
	Status               string   `json:"status"`
	Title                string   `json:"title"`
	Parent               string   `json:"parent,omitempty"`
	Priority             string   `json:"priority,omitempty"`
	Scope                []string `json:"scope,omitempty"`
	EstComplexity        string   `json:"estimated_complexity,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	AssignedWorker       string   `json:"assigned_worker,omitempty"`
}

// ExpiredClaim is the expired_claims adjunct. Rows stay out of issues (N2).
type ExpiredClaim struct {
	ID                         string `json:"id"`
	Title                      string `json:"title"`
	Status                     string `json:"status"`
	ClaimedBy                  string `json:"claimed_by"`
	ClaimedAt                  int64  `json:"claimed_at"`
	LastHeartbeat              int64  `json:"last_heartbeat"`
	ClaimTTL                   int    `json:"claim_ttl"`
	LastClaimingWorkerActivity int64  `json:"last_claiming_worker_activity,omitempty"`
}

// readyIssueRows maps compute-ready entries to envelope rows.
func readyIssueRows(entries []ready.ReadyEntry) []ReadyIssue {
	rows := make([]ReadyIssue, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, ReadyIssue{
			ID:                   e.Issue,
			Type:                 e.Type,
			Status:               "open",
			Title:                e.Title,
			Parent:               e.Parent,
			Priority:             e.Priority,
			Scope:                e.Scope,
			EstComplexity:        e.EstComplexity,
			RequiresConfirmation: e.RequiresConfirmation,
			AssignedWorker:       e.AssignedWorker,
		})
	}
	return rows
}

// ReadyWaveIDs maps partitioned waves to id-only adjunct groups.
func ReadyWaveIDs(waves [][]ready.ReadyEntry) [][]string {
	groups := make([][]string, 0, len(waves))
	for _, wave := range waves {
		ids := make([]string, 0, len(wave))
		for _, e := range wave {
			ids = append(ids, e.Issue)
		}
		groups = append(groups, ids)
	}
	return groups
}

// expiredClaimRows maps expired-claim entries to the expired_claims adjunct.
func expiredClaimRows(claims []ready.ExpiredClaimEntry) []ExpiredClaim {
	rows := make([]ExpiredClaim, 0, len(claims))
	for _, c := range claims {
		rows = append(rows, ExpiredClaim{
			ID:                         c.Issue,
			Title:                      c.Title,
			Status:                     c.Status,
			ClaimedBy:                  c.ClaimedBy,
			ClaimedAt:                  c.ClaimedAt,
			LastHeartbeat:              c.LastHeartbeat,
			ClaimTTL:                   c.ClaimTTL,
			LastClaimingWorkerActivity: c.LastClaimingWorkerActivity,
		})
	}
	return rows
}

// readyEmptyReason names why a zero-length ready payload is empty.
func readyEmptyReason(parent, assignedTo string, expiredN int) string {
	switch {
	case parent != "" && assignedTo != "":
		return fmt.Sprintf("no issues match --parent %s and --assigned-to %s", parent, assignedTo)
	case parent != "":
		return fmt.Sprintf("no issues match --parent %s", parent)
	case assignedTo != "":
		return fmt.Sprintf("no issues match --assigned-to %s", assignedTo)
	case expiredN > 0:
		return ReadyEmptyExpiredHelp
	default:
		return ReadyEmptyHelp
	}
}

// ReadyHelp is the trailing help for a ready envelope.
func ReadyHelp(n int, waves bool, expiredN int, parent, assignedTo string) []string {
	help := make([]string, 0, 4)
	switch {
	case n == 0:
		help = append(help, readyEmptyReason(parent, assignedTo, expiredN))
	case waves:
		help = append(help, ReadyWavesHelp)
	default:
		help = append(help, ReadyClaimHelp)
	}
	help = append(help, ReadyShowHelp)
	if expiredN > 0 {
		help = append(help, ReadyExpiredHelp)
	}
	return help
}

// WriteReadyEnvelope emits the compact agent ready object
// {count,issues,expired_claims,help} and optional waves adjunct.
// This is the live arm ready json/agent path.
func WriteReadyEnvelope(
	w io.Writer,
	entries []ready.ReadyEntry,
	waves [][]ready.ReadyEntry,
	includeWaves bool,
	expired []ready.ExpiredClaimEntry,
	parent, assignedTo string,
) error {
	rows := readyIssueRows(entries)
	env, err := NewEnvelope("issues", rows, ReadyHelp(len(rows), includeWaves, len(expired), parent, assignedTo))
	if err != nil {
		return err
	}
	if includeWaves {
		if err := env.AddAdjunct("waves", ReadyWaveIDs(waves)); err != nil {
			return err
		}
	}
	if err := env.AddAdjunct("expired_claims", expiredClaimRows(expired)); err != nil {
		return err
	}
	return WriteEnvelope(w, env)
}
