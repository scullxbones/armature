package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/spf13/cobra"
)

const codePromotionReady1 = "PROMOTION-READY-1"

func init() {
	armerrors.Register(codePromotionReady1)
}

func newPromotionReadyCmd() *cobra.Command {
	var issueIDs []string
	var base, head, actionsRunID string

	cmd := &cobra.Command{
		Use:   "promotion-ready",
		Short: "Verify a delivery is promotion-ready (ADR 0026 poka-yoke)",
		Long: `Verify-only pre-merge check for ID-scoped deliveries (ADR 0026 N1).

Refuses empty contracts and unknown scopes (R4), missing CI full gate
evidence at the current head (R2), and assessments that do not match the
delivery (base, tip) for that head (R3). Never authors an assessment and
never calls a model.

--actions-run-id, when set, requires the R2 evidence to carry that GitHub
Actions run id. PromotionReadyEvidence itself is unchanged.

promotion-ready scope derives --issue and --base from ops state for a PR
head and base SHA. It does not read the PR body.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapPromotionReadyError(runPromotionReady(cmd, issueIDs, base, head, actionsRunID))
		},
	}

	cmd.Flags().StringArrayVar(&issueIDs, "issue", nil, "issue ID to verify (repeatable; required)")
	cmd.Flags().StringVar(&base, "base", "", "delivery base SHA (required)")
	cmd.Flags().StringVar(&head, "head", "", "delivery tip / current head SHA (required)")
	cmd.Flags().StringVar(&actionsRunID, "actions-run-id", "", "require R2 evidence recorded by this GitHub Actions run")
	cmd.AddCommand(newPromotionScopeCmd())
	return cmd
}

func runPromotionReady(cmd *cobra.Command, issueIDs []string, base, head, actionsRunID string) error {
	if len(issueIDs) == 0 {
		return fmt.Errorf("--issue is required")
	}
	if base == "" {
		return fmt.Errorf("--base is required")
	}
	if head == "" {
		return fmt.Errorf("--head is required")
	}

	ctx := currentCtx(cmd)
	store := newSnapshotStore(ctx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	issues := make(map[string]promotion.ReadyIssue, len(issueIDs))
	for _, id := range issueIDs {
		issuePtr, ok := snap.Issues[id]
		if !ok || issuePtr == nil {
			continue
		}
		issue := *issuePtr
		facts := promotionIssueFacts(issue)
		issues[id] = promotion.ReadyIssue{
			DefinitionOfDone: issue.DefinitionOfDone,
			Acceptance:       issue.Acceptance,
			Assessments:      facts.AssessmentAttestations,
		}
	}

	evs, err := ops.ReadAllGateEvidence(filepath.Join(ctx.IssuesDir, "ops"))
	if err != nil {
		return fmt.Errorf("read gate evidence: %w", err)
	}
	stopHistory, err := ops.ReadAllPromotionStops(filepath.Join(ctx.IssuesDir, "ops"))
	if err != nil {
		return fmt.Errorf("read promotion stops: %w", err)
	}
	graph := promotionGraphFromIssues(snap.Issues)

	if err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA:      base,
		HeadSHA:      head,
		IssueIDs:     issueIDs,
		Issues:       issues,
		GateEvidence: evs,
		Stops:        promotion.ActiveStops(stopHistory),
		Graph:        graph,
		ActionsRunID: actionsRunID,
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "promotion-ready: ok for %s at %s\n", strings.Join(issueIDs, ","), head)
	return nil
}

func newPromotionScopeCmd() *cobra.Command {
	var head, boundary, boundaryRef string
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Derive the PR delivery promotion-ready will verify",
		Long: `Derive --issue IDs and --base for a pull request from ops state.

A done leaf is in scope when its recorded tip is an ancestor of --head
(including --head) and is not an ancestor of --boundary. --boundary is the
pull request base SHA. --base is the leaf's recorded delivery base. Several
leaves that share a base are one delivery. A missing tip object is outside
the range. The PR body is not read.

kind=no-scope means arm land has nothing to promote from this range.
A stacked base ref (not the integration branch) uses the same pass/fail
rule on each recorded delivery base.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapPromotionReadyError(runPromotionScope(cmd, head, boundary, boundaryRef))
		},
	}
	cmd.Flags().StringVar(&head, "head", "", "pull request head SHA (required)")
	cmd.Flags().StringVar(&boundary, "boundary", "", "pull request base SHA; tips already on it are out of scope (required)")
	cmd.Flags().StringVar(&boundaryRef, "boundary-ref", "", "pull request base ref name; not the integration branch means stacked")
	return cmd
}

func runPromotionScope(cmd *cobra.Command, head, boundary, boundaryRef string) error {
	if strings.TrimSpace(head) == "" {
		return fmt.Errorf("--head is required")
	}
	if strings.TrimSpace(boundary) == "" {
		return fmt.Errorf("--boundary is required")
	}
	ctx := currentCtx(cmd)
	git := adapters.New(ctx.RepoPath)
	headSHA, err := git.ResolveRevision(head)
	if err != nil {
		return fmt.Errorf("resolve --head: %w", err)
	}
	boundarySHA, err := git.ResolveRevision(boundary)
	if err != nil {
		return fmt.Errorf("resolve --boundary: %w", err)
	}
	store := newSnapshotStore(ctx)
	snap, err := store.LoadReadOnly(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	issues := make([]scopeIssue, 0, len(snap.Issues))
	for id, issuePtr := range snap.Issues {
		if issuePtr == nil {
			continue
		}
		issue := *issuePtr
		facts := promotionIssueFacts(issue)
		issues = append(issues, scopeIssue{
			ID:          id,
			Status:      issue.Status,
			Base:        issue.Base,
			Tip:         issue.Tip,
			Children:    append([]string(nil), issue.Children...),
			Assessments: facts.AssessmentAttestations,
		})
	}
	in := scopeInput{
		Head:           headSHA,
		Boundary:       boundarySHA,
		BoundaryRef:    strings.TrimSpace(boundaryRef),
		IntegrationRef: ctx.Config.IntegrationBranchOrDefault(),
		Issues:         issues,
		Ancestor:       scopeAncestor(git),
	}
	res, err := deriveScope(in)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), formatScope(in, res))
	return nil
}

func scopeAncestor(git *adapters.Client) scopeAncestorFunc {
	return func(ancestor, descendant string) (bool, error) {
		if _, err := git.ResolveRevision(ancestor); err != nil {
			return false, nil
		}
		if _, err := git.ResolveRevision(descendant); err != nil {
			return false, nil
		}
		return git.IsAncestor(ancestor, descendant)
	}
}

func mapPromotionReadyError(err error) error {
	if err == nil {
		return nil
	}
	if mapped, done := mappedCommandFailure(err); done {
		return mapped
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "--issue is required"),
		strings.Contains(msg, "--base is required"),
		strings.Contains(msg, "--head is required"),
		strings.Contains(msg, "--boundary is required"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm promotion-ready --help"}, err)
	case strings.Contains(msg, "jidoka stop"):
		return armerrors.Wrap(codePromotionReady1, msg, []string{
			"arm promotion-stop clear --issue <id> --msg <root-cause>",
			"arm doctor",
		}, err)
	default:
		return armerrors.Wrap(codePromotionReady1, msg, []string{
			"arm review prepare --issue <id> --base <sha> --head <sha>",
			"arm review record --issue <id> --assessment <file> --bundle <file>",
			"arm gate run full",
		}, err)
	}
}
