package main

import (
	"fmt"
	"io"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	claimPkg "github.com/scullxbones/armature/internal/claim"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/tui"
	readytui "github.com/scullxbones/armature/internal/tui/ready"
	"github.com/spf13/cobra"
)

const (
	readyShowHelp         = output.ReadyShowHelp
	readyClaimHelp        = output.ReadyClaimHelp
	readyEmptyHelp        = output.ReadyEmptyHelp
	readyEmptyExpiredHelp = output.ReadyEmptyExpiredHelp
	readyWavesHelp        = output.ReadyWavesHelp
	readyExpiredHelp      = output.ReadyExpiredHelp
	readyExplainHelp      = "these issues are open but not ready to claim"
	readyExplainEmptyHelp = "no open issues are excluded from the ready queue"
)

type readyIssueRow = output.ReadyIssue

type readyExplainRow struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

func readyExplainRows(index materialize.Index, notReady map[string]string) []readyExplainRow {
	ids := make([]string, 0, len(notReady))
	for id := range notReady {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rows := make([]readyExplainRow, 0, len(ids))
	for _, id := range ids {
		entry := index[id]
		rows = append(rows, readyExplainRow{
			ID:     id,
			Type:   entry.Type,
			Status: entry.Status,
			Title:  entry.Title,
			Reason: notReady[id],
		})
	}
	return rows
}

func explainHelp(n int) []string {
	help := make([]string, 0, 2)
	if n == 0 {
		help = append(help, readyExplainEmptyHelp)
	} else {
		help = append(help, readyExplainHelp)
	}
	help = append(help, readyShowHelp)
	return help
}

func writeReadyExplainEnvelope(w io.Writer, index materialize.Index, notReady map[string]string) error {
	rows := readyExplainRows(index, notReady)
	return writeCommandEnvelope(w, "issues", rows, explainHelp(len(rows)), nil)
}

func writeReadyHome(cmd *cobra.Command, emptyReason string) error {
	if emptyReason != "" {
		help := []string{
			"not an Armature repository: " + emptyReason,
			"run arm bootstrap in a git repository",
		}
		return writeCommandEnvelope(cmd.OutOrStdout(), "issues", []readyIssueRow{}, help, nil)
	}
	ctx := currentCtx(cmd)
	store := newSnapshotStore(ctx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return mapReadyError(fmt.Errorf("load snapshot: %w", err))
	}
	emitSnapWarnings(cmd.ErrOrStderr(), snap.Warnings)
	entries := ready.ComputeReady(snap.Index, snap.Issues, "", nowEpoch())
	expiredClaims := ready.ExpiredClaims(snap.Issues, time.Now())
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if isStructuredFormat(format) || tui.IsNonInteractive() {
		return output.WriteReadyEnvelope(cmd.OutOrStdout(), readyOutputIssues(entries), nil, false, readyOutputExpired(expiredClaims), "", "")
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
	} else if err := output.RenderReady(cmd.OutOrStdout(), readyOutputIssues(entries)); err != nil {
		return err
	}
	return output.RenderExpiredClaims(cmd.OutOrStdout(), readyOutputExpired(expiredClaims))
}

func newReadyCmd() *cobra.Command {
	var workerID string
	var filterParent string
	var assignedTo string
	var explain bool
	var waves bool

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show tasks ready to be claimed",
		Long: `Display all issues that are ready to be claimed by a worker.

An issue is ready when it has no unmet blocking dependencies and its status is "open".
This command shows a prioritized list of tasks available for work, optionally filtered
to a specific worker or a subtree of issues. Use --format json for automation.`,
		Example: `  # Show all ready tasks in interactive mode
  $ arm ready

  # Show ready tasks filtered for a specific worker
  $ arm ready --worker alice-worker

  # Show ready tasks scoped to a specific story subtree
  $ arm ready --parent STORY-ID

  # Show ready tasks in JSON format (suitable for agents)
  $ arm ready --format json

  # Diagnose why open tasks are not in the ready queue
  $ arm ready --explain`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapReadyError(err) }()
			ctx := currentCtx(cmd)

			store := newSnapshotStore(ctx)
			snap, err := store.Load(cmd.Context())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			emitSnapWarnings(cmd.ErrOrStderr(), snap.Warnings)
			index := snap.Index
			issues := snap.Issues

			if explain {
				notReady := ready.ExplainNotReady(index, issues, nowEpoch())
				format, _ := cmd.Root().PersistentFlags().GetString("format")
				if isStructuredFormat(format) || tui.IsNonInteractive() {
					return writeReadyExplainEnvelope(cmd.OutOrStdout(), index, notReady)
				}
				ids := make([]string, 0, len(notReady))
				for id := range notReady {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				for _, id := range ids {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", id, notReady[id])
				}
				return nil
			}

			entries := ready.ComputeReady(index, issues, workerID, nowEpoch())
			expiredClaims := ready.ExpiredClaims(issues, time.Now())

			entries = ready.FilterByAssignedTo(entries, assignedTo)
			if assignedTo != "" {
				expiredClaims = filterExpiredClaimsByIssueAssignedWorker(expiredClaims, issues, assignedTo)
			}

			if filterParent != "" {
				descendants := ready.CollectDescendants(filterParent, index)
				filtered := entries[:0]
				for _, e := range entries {
					if descendants[e.Issue] {
						filtered = append(filtered, e)
					}
				}
				entries = filtered

				filteredExpired := expiredClaims[:0]
				for _, e := range expiredClaims {
					if descendants[e.Issue] {
						filteredExpired = append(filteredExpired, e)
					}
				}
				expiredClaims = filteredExpired
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			switch {
			case isStructuredFormat(format) || tui.IsNonInteractive():
				var wavesData [][]output.ReadyIssue
				if waves {
					for _, wave := range ready.PartitionWaves(entries, index) {
						wavesData = append(wavesData, readyOutputIssues(wave))
					}
				}
				if err := output.WriteReadyEnvelope(cmd.OutOrStdout(), readyOutputIssues(entries), wavesData, waves, readyOutputExpired(expiredClaims), filterParent, assignedTo); err != nil {
					return err
				}
			case tui.IsInteractive():
				selected, err := runReadyTUI(entries)
				if err != nil {
					return err
				}
				if selected != "" {
					state := mustState(cmd)
					ctx := state.ctx
					workerID, logPath, err := resolveWorkerAndLog(ctx)
					if err != nil {
						return err
					}
					ttl := 60
					if ctx.Config.DefaultTTL > 0 {
						ttl = int(ctx.Config.DefaultTTL)
					}
					token, err := newClaimToken()
					if err != nil {
						return err
					}
					op, err := claimPkg.NewClaimOp(selected, workerID, nowEpoch(), ttl, "", token)
					if err != nil {
						return err
					}
					if err := appendHighStakesOp(state, logPath, op); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s\n", selected)
				}
				return nil
			default:
				if len(entries) == 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
				} else if err := output.RenderReady(cmd.OutOrStdout(), readyOutputIssues(entries)); err != nil {
					return err
				}
				if err := output.RenderExpiredClaims(cmd.OutOrStdout(), readyOutputExpired(expiredClaims)); err != nil {
					return err
				}
				return nil
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workerID, "worker", "", "worker ID for assignment-aware sorting")
	cmd.Flags().StringVar(&filterParent, "parent", "", "filter to descendants of this issue ID")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "filter to tasks assigned to this worker ID")
	cmd.Flags().BoolVar(&explain, "explain", false, "diagnose why open tasks are not in the ready queue")
	cmd.Flags().BoolVar(&waves, "waves", false, "partition ready entries into scope-disjoint waves (JSON/agent output only)")
	return cmd
}

func readyOutputIssues(entries []ready.ReadyEntry) []output.ReadyIssue {
	rows := make([]output.ReadyIssue, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, output.ReadyIssue{
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

func readyOutputExpired(claims []ready.ExpiredClaimEntry) []output.ExpiredClaim {
	rows := make([]output.ExpiredClaim, 0, len(claims))
	for _, c := range claims {
		rows = append(rows, output.ExpiredClaim{
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

func filterExpiredClaimsByIssueAssignedWorker(
	expiredClaims []ready.ExpiredClaimEntry,
	issues map[string]*materialize.Issue,
	assignedWorker string,
) []ready.ExpiredClaimEntry {
	filtered := expiredClaims[:0]
	for _, e := range expiredClaims {
		issue := issues[e.Issue]
		if issue != nil && issue.AssignedWorker == assignedWorker {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

const codeReady1 = "READY-1"

func init() {
	armerrors.Register(codeReady1)
}

func runReadyTUI(entries []ready.ReadyEntry) (string, error) {
	m := readytui.New(entries)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}
	final, ok := finalModel.(readytui.Model)
	if !ok {
		return "", fmt.Errorf("unexpected model type from TUI")
	}
	return final.Selected(), nil
}

func mapReadyError(err error) error {
	if mapped, done := mappedCommandFailure(err); done {
		return mapped
	}
	if isLocalArmatureTipPublishError(err) {
		return wrapOpsPublishFailure(codeReady1, err)
	}
	return armerrors.Wrap(codeReady1, err.Error(), []string{"arm doctor"}, err)
}
