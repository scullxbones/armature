package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var targetBranch string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Classify done leaves and promote those whose delivery is on the target",
		Long: `Run the same promotion check as arm merged for every done leaf.

A pass appends merged (and tears down the delivery ref and worktree). A legacy
done with no delivery record is listed and does not fail. Exit is non-zero only
when a delivery is on the target and the assessment is missing, or a recorded
delivery cannot be checked. --dry-run writes nothing. Default --into is each
issue's recorded integration branch (else config).`,
		Example: `  # Classify done leaves against recorded integration branches
  $ arm sync

  # Override the integration branch for every leaf
  $ arm sync --into main

  # Preview classifications without writing ops
  $ arm sync --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoneLeafPromotion(cmd, targetBranch, dryRun, true)
		},
	}

	cmd.Flags().StringVar(&targetBranch, "into", "", "override integration branch (default: recorded per issue, else config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "classify without writing ops")
	return cmd
}

type classifiedLeaf struct {
	Issue        materialize.Issue
	Result       promotion.Result
	Integration  string // effective Evaluate target (--into, else recorded, else config)
	ExplicitInto bool   // true when --into overrode the recorded integration branch
}

func runDoneLeafPromotion(cmd *cobra.Command, into string, dryRun, failExit bool) error {
	ctx := currentCtx(cmd)
	store := newSnapshotStore(ctx)
	loadCtx := cmd.Context()
	if loadCtx == nil {
		loadCtx = context.Background()
	}
	var snap *snapshot.Snapshot
	var err error
	if dryRun {
		snap, err = store.LoadReadOnly(loadCtx)
	} else {
		snap, err = store.Load(loadCtx)
	}
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	emitSnapWarnings(cmd.ErrOrStderr(), snap.Warnings)

	allOps, err := readAllOpsFromDir(filepath.Join(ctx.IssuesDir, "ops"))
	if err != nil {
		return fmt.Errorf("read ops: %w", err)
	}

	git := adapters.New(ctx.RepoPath)
	items := classifyDoneLeaves(git, snap.Issues, allOps, into, ctx.Config.IntegrationBranchOrDefault())

	if !dryRun {
		if persistErr := persistClassifications(ctx, git, items, cmd.ErrOrStderr()); persistErr != nil {
			if failExit {
				return persistErr
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", persistErr)
		}
		if teardownErr := teardownMergedLeftovers(ctx, git, snap.Issues, allOps, cmd.ErrOrStderr()); teardownErr != nil {
			if failExit {
				return teardownErr
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", teardownErr)
		}
	}

	// Rows reflect post-persist outcomes so a locked promote skip cannot
	// still report status=merged / kind=promote.
	rows := syncRows(items)
	if err := output.WriteSyncEnvelope(cmd.OutOrStdout(), rows); err != nil {
		return err
	}
	if failExit && syncFails(rows) {
		return protocolExitError{code: 1}
	}
	return nil
}

func classifyDoneLeaves(git *adapters.Client, issues map[string]*materialize.Issue, allOps []ops.Op, into, configDefault string) []classifiedLeaf {
	ids := make([]string, 0, len(issues))
	for id, issue := range issues {
		if issue == nil || issue.Status != ops.StatusDone || len(issue.Children) > 0 {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	explicitInto := strings.TrimSpace(into) != ""
	out := make([]classifiedLeaf, 0, len(ids))
	for _, id := range ids {
		issue := *issues[id]
		integration := strings.TrimSpace(into)
		if integration == "" {
			integration = issue.IntegrationBranch
			if integration == "" {
				integration = configDefault
			}
		}
		res := promotion.Evaluate(git, promotion.Input{
			Issue:       issue,
			PriorOps:    allOps,
			Integration: integration,
		})
		out = append(out, classifiedLeaf{
			Issue:        issue,
			Result:       res,
			Integration:  integration,
			ExplicitInto: explicitInto,
		})
	}
	return out
}

func syncRows(items []classifiedLeaf) []output.SyncIssue {
	rows := make([]output.SyncIssue, 0, len(items))
	for _, item := range items {
		row := output.SyncIssue{
			ID:     item.Issue.ID,
			Type:   item.Issue.Type,
			Status: item.Issue.Status,
			Title:  item.Issue.Title,
			Kind:   item.Result.Kind,
		}
		if item.Result.Kind == promotion.KindPromote {
			row.Status = ops.StatusMerged
		}
		if !item.Result.Promote && item.Result.Recovery != "" {
			row.NextAction = item.Result.Recovery
		}
		rows = append(rows, row)
	}
	return rows
}

func syncFails(rows []output.SyncIssue) bool {
	for _, row := range rows {
		if row.Kind == promotion.KindMissingAssessment || row.Kind == promotion.KindCheckFailed {
			return true
		}
	}
	return false
}

func persistClassifications(ctx *config.Context, git *adapters.Client, items []classifiedLeaf, errWriter io.Writer) error {
	needsWrite := false
	for _, item := range items {
		if item.Result.AppendCheck || item.Result.Promote {
			needsWrite = true
			break
		}
	}
	if !needsWrite {
		return nil
	}
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		item := &items[i]
		if item.Result.AppendCheck {
			checkOp := ops.Op{
				Type:      ops.OpPromotionCheck,
				TargetID:  item.Issue.ID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   item.Result.CheckPayload,
			}
			if err := appendOp(ctx, logPath, checkOp); err != nil {
				return err
			}
		}
		if !item.Result.Promote {
			continue
		}
		wrote, appendErr := appendMergedIfCurrent(
			ctx, git, logPath, workerID, item.Issue, item.Integration, item.ExplicitInto, "",
		)
		if appendErr != nil {
			return appendErr
		}
		if !wrote {
			if refreshErr := refreshClassifiedLeaf(ctx, git, item); refreshErr != nil {
				return refreshErr
			}
			continue
		}
		if err := teardownDeliveryArtifacts(ctx.RepoPath, git, item.Issue, errWriter); err != nil {
			return err
		}
	}
	return nil
}

func refreshClassifiedLeaf(ctx *config.Context, git *adapters.Client, item *classifiedLeaf) error {
	live, allOps, err := replayIssueOps(ctx.IssuesDir, item.Issue.ID)
	if err != nil {
		return err
	}
	if live == nil {
		return nil
	}
	item.Issue = *live
	if live.Status != ops.StatusDone {
		item.Result = promotion.Result{
			IssueID: live.ID,
			Kind:    promotion.KindNotDone,
		}
		return nil
	}
	if len(live.Children) > 0 {
		item.Result = promotion.Result{
			IssueID:  live.ID,
			Kind:     promotion.KindNotLeaf,
			Recovery: promotion.RecoveryArgv(live.ID, promotion.KindNotLeaf),
		}
		return nil
	}
	integ := item.Integration
	if !item.ExplicitInto {
		integ = live.IntegrationBranch
		if integ == "" {
			integ = item.Integration
		}
		item.Integration = integ
	}
	item.Result = promotion.Evaluate(git, promotion.Input{
		Issue:       *live,
		PriorOps:    allOps,
		Integration: integ,
	})
	return nil
}

// teardownMergedLeftovers finishes delivery-ref / worktree cleanup for issues
// already promoted via ADR 0022 when a prior sync append succeeded but
// teardown failed. Legacy merged issues without recorded promotion evidence
// are left alone (ADR 0022: already-merged stay put).
func teardownMergedLeftovers(ctx *config.Context, git *adapters.Client, issues map[string]*materialize.Issue, allOps []ops.Op, errWriter io.Writer) error {
	ids := make([]string, 0, len(issues))
	for id, issue := range issues {
		if issue == nil || issue.Status != ops.StatusMerged {
			continue
		}
		if !recordedPromotionEvidence(allOps, id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := teardownDeliveryArtifacts(ctx.RepoPath, git, *issues[id], errWriter); err != nil {
			return err
		}
	}
	return nil
}

func recordedPromotionEvidence(allOps []ops.Op, issueID string) bool {
	for _, op := range allOps {
		if op.TargetID != issueID {
			continue
		}
		switch op.Type {
		case ops.OpPromotionCheck:
			if op.Payload.Result == promotion.KindPromote {
				return true
			}
		case ops.OpTransition:
			if op.Payload.To == ops.StatusMerged &&
				(op.Payload.TargetSHA != "" || op.Payload.CombinedPatchID != "" || op.Payload.MatchedCommit != "") {
				return true
			}
		}
	}
	return false
}
