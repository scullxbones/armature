package main

import (
	"fmt"
	"path/filepath"
	"strings"

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
	var base, head string

	cmd := &cobra.Command{
		Use:   "promotion-ready",
		Short: "Verify a delivery is promotion-ready (ADR 0026 poka-yoke)",
		Long: `Verify-only pre-merge check for ID-scoped deliveries (ADR 0026 N1).

Refuses empty contracts and unknown scopes (R4), missing CI full gate
evidence at the current head (R2), and assessments that do not match the
delivery (base, tip) for that head (R3). Never authors an assessment and
never calls a model.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapPromotionReadyError(runPromotionReady(cmd, issueIDs, base, head))
		},
	}

	cmd.Flags().StringArrayVar(&issueIDs, "issue", nil, "issue ID to verify (repeatable; required)")
	cmd.Flags().StringVar(&base, "base", "", "delivery base SHA (required)")
	cmd.Flags().StringVar(&head, "head", "", "delivery tip / current head SHA (required)")
	return cmd
}

func runPromotionReady(cmd *cobra.Command, issueIDs []string, base, head string) error {
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
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "promotion-ready: ok for %s at %s\n", strings.Join(issueIDs, ","), head)
	return nil
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
		strings.Contains(msg, "--head is required"):
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
