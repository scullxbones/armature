package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

const codeLand1 = "LAND-1"

func init() {
	armerrors.Register(codeLand1)
}

func newLandCmd() *cobra.Command {
	var pr string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "land",
		Short: "Apply stored promotion for a merged PR (ADR 0026 N1)",
		Long: `Promote done leaves scoped to GitHub PR N after merge.

Fetches refs/pull/N/head so the delivery tip remains reachable after a squash
(R1). Applies the ADR 0022 matcher against the integration branch and appends
merged (with the matched squash SHA) when done + attestation are already
stored. --dry-run writes nothing.`,
		Example: `  # Apply promotion after PR 464 merges
  $ arm land --pr 464

  # Early warning before merge (no ops written)
  $ arm land --pr 464 --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapLandError(runLand(cmd, pr, dryRun))
		},
	}

	cmd.Flags().StringVar(&pr, "pr", "", "PR number (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "classify without writing ops")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}

func runLand(cmd *cobra.Command, prFlag string, dryRun bool) error {
	pr, err := normalizePRNumber(prFlag)
	if err != nil {
		return err
	}

	ctx := currentCtx(cmd)
	git := adapters.New(ctx.RepoPath)
	prHead, err := git.FetchPullHead(pr)
	if err != nil {
		return fmt.Errorf("fetch refs/pull/%s/head: %w", pr, err)
	}

	store := newSnapshotStore(ctx)
	loadCtx := cmd.Context()
	if loadCtx == nil {
		loadCtx = context.Background()
	}
	var snap *snapshot.Snapshot
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

	items, err := classifyPRLeaves(git, snap.Issues, allOps, pr, prHead, ctx.Config.IntegrationBranchOrDefault())
	if err != nil {
		return err
	}

	if !dryRun {
		if persistErr := persistClassifications(ctx, git, items, cmd.ErrOrStderr()); persistErr != nil {
			return persistErr
		}
		if teardownErr := teardownMergedLeftovers(ctx, git, snap.Issues, allOps, cmd.ErrOrStderr()); teardownErr != nil {
			return teardownErr
		}
	}

	rows := syncRows(items)
	if err := output.WriteSyncEnvelope(cmd.OutOrStdout(), rows); err != nil {
		return err
	}
	if syncFails(rows) {
		if !dryRun {
			graph := promotionGraphFromIssues(snap.Issues)
			if tripErr := tripJidokaOnFailedRows(cmd, pr, rows, graph); tripErr != nil {
				return tripErr
			}
		}
		return protocolExitError{code: 1}
	}
	return nil
}

func normalizePRNumber(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "#")
	if i := strings.LastIndex(s, "/pull/"); i >= 0 {
		s = s[i+len("/pull/"):]
	}
	if i := strings.LastIndex(s, "/pulls/"); i >= 0 {
		s = s[i+len("/pulls/"):]
	}
	if slash := strings.IndexByte(s, '/'); slash >= 0 {
		s = s[:slash]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("--pr is required")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("invalid --pr %q: must be a PR number", raw)
		}
	}
	return s, nil
}

func classifyPRLeaves(
	git *adapters.Client,
	issues map[string]*materialize.Issue,
	allOps []ops.Op,
	pr, prHead, configDefault string,
) ([]classifiedLeaf, error) {
	ids := make([]string, 0, len(issues))
	for id, issue := range issues {
		if issue == nil || issue.Status != ops.StatusDone || len(issue.Children) > 0 {
			continue
		}
		if issue.Tip == "" {
			continue
		}
		ok, err := deliveryBelongsToPRHead(git, issue.Tip, prHead)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]classifiedLeaf, 0, len(ids))
	for _, id := range ids {
		issue := *issues[id]
		integration := issue.IntegrationBranch
		if integration == "" {
			integration = configDefault
		}
		res := promotion.Evaluate(git, promotion.Input{
			Issue:       promotionIssueFacts(issue),
			PriorOps:    allOps,
			Integration: integration,
			PR:          pr,
		})
		out = append(out, classifiedLeaf{
			Issue:        issue,
			Result:       res,
			Integration:  integration,
			ExplicitInto: false,
			PR:           pr,
		})
	}
	return out, nil
}

func deliveryBelongsToPRHead(git *adapters.Client, tip, prHead string) (bool, error) {
	if tip == prHead {
		return true, nil
	}
	return git.IsAncestor(tip, prHead)
}

func mapLandError(err error) error {
	if err == nil {
		return nil
	}
	if mapped, done := mappedCommandFailure(err); done {
		return mapped
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "required flag"),
		strings.Contains(msg, "--pr is required"),
		strings.Contains(msg, "invalid --pr"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm land --help"}, err)
	default:
		return armerrors.Wrap(codeLand1, msg, []string{
			"arm land --pr <N>",
			"arm sync --dry-run",
			"arm doctor",
		}, err)
	}
}
