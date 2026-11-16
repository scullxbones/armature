package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/spf13/cobra"
)

const codePromotionStop1 = "PROMOTION-STOP-1"

func init() {
	armerrors.Register(codePromotionStop1)
}

// issueCreator opens a GitHub issue; tests inject a fake.
type issueCreator func(ctx context.Context, owner, repo, title, body string, labels []string) (externalref.CreatedIssue, error)

var jidokaCreateIssue issueCreator

func newPromotionStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promotion-stop",
		Short: "Manage jidoka tree-scoped promotion stops (ADR 0026)",
		Long: `Trip and clear tree-scoped promotion stops (ADR 0026 jidoka).

A land/sweep failure arms a stop for the affected issue tree and opens one
andon GitHub issue. Clearing requires a root-cause note.`,
	}
	cmd.AddCommand(newPromotionStopClearCmd())
	return cmd
}

func newPromotionStopClearCmd() *cobra.Command {
	var issueID, msg string
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear a jidoka stop with a root-cause note",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mapPromotionStopError(runPromotionStopClear(cmd, issueID, msg))
		},
	}
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID covered by the stop (required)")
	cmd.Flags().StringVar(&msg, "msg", "", "root-cause note (required)")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("msg")
	return cmd
}

func runPromotionStopClear(cmd *cobra.Command, issueID, msg string) error {
	issueID = strings.TrimSpace(issueID)
	msg = strings.TrimSpace(msg)
	if issueID == "" {
		return fmt.Errorf("--issue is required")
	}
	if msg == "" {
		return fmt.Errorf("root-cause note is required (--msg)")
	}

	cfg := currentCtx(cmd)
	store := newSnapshotStore(cfg)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	graph := promotionGraphFromIssues(snap.Issues)

	history, err := ops.ReadAllPromotionStops(filepath.Join(cfg.IssuesDir, "ops"))
	if err != nil {
		return fmt.Errorf("read promotion stops: %w", err)
	}
	active := promotion.ActiveStops(history)
	stop := promotion.StopCovering(active, graph, []string{issueID})
	if stop == nil {
		return fmt.Errorf("no active jidoka stop covers %s", issueID)
	}

	workerID, logPath, err := resolveWorkerAndLog(cfg)
	if err != nil {
		return err
	}
	noteID := fmt.Sprintf("jidoka-rc-%d", time.Now().UnixNano())
	noteMsg := "jidoka-root-cause: " + msg
	noteOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  stop.TreeRoot,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{Msg: noteMsg, NoteID: noteID},
	}
	if err := appendOp(cfg, logPath, noteOp); err != nil {
		return err
	}
	if err := ops.AppendPromotionStop(logPath, workerID, ops.PromotionStop{
		Action:   ops.PromotionStopClear,
		TreeRoot: stop.TreeRoot,
		IssueID:  stop.IssueID,
		PR:       stop.PR,
		Step:     stop.Step,
		Kind:     stop.Kind,
		NoteID:   noteID,
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "promotion-stop: cleared tree %s (note %s)\n", stop.TreeRoot, noteID)
	return nil
}

func mapPromotionStopError(err error) error {
	if err == nil {
		return nil
	}
	if mapped, done := mappedCommandFailure(err); done {
		return mapped
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "--issue is required"),
		strings.Contains(msg, "root-cause note is required"),
		strings.Contains(msg, "required flag"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm promotion-stop clear --help"}, err)
	default:
		return armerrors.Wrap(codePromotionStop1, msg, []string{
			"arm promotion-stop clear --issue <id> --msg <root-cause>",
			"arm note --issue <id> --msg <text>",
			"arm doctor",
		}, err)
	}
}

// tripJidokaOnFailedRows arms a tree stop for the first failing sync row and
// opens exactly one andon (or a dogfood finding on repeated kind).
func tripJidokaOnFailedRows(
	cmd *cobra.Command,
	pr string,
	rows []output.SyncIssue,
	graph promotion.Graph,
) error {
	var failed *output.SyncIssue
	for i := range rows {
		row := &rows[i]
		if row.Kind == promotion.KindMissingAssessment || row.Kind == promotion.KindCheckFailed {
			failed = row
			break
		}
	}
	if failed == nil {
		return nil
	}

	cfg := currentCtx(cmd)
	opsDir := filepath.Join(cfg.IssuesDir, "ops")
	history, err := ops.ReadAllPromotionStops(opsDir)
	if err != nil {
		return fmt.Errorf("read promotion stops: %w", err)
	}

	kind := failed.Kind
	step := "land:" + kind
	plan := promotion.PlanTrip(graph, history, failed.ID, kind)

	workerID, logPath, err := resolveWorkerAndLog(cfg)
	if err != nil {
		return err
	}

	owner, repoName := githubOwnerRepo(envFrom(cmd))
	andonURL := ""
	dogfood := plan.OpenDogfood
	create := jidokaCreateIssue
	if create == nil {
		create = issueCreatorFromEnv(envFrom(cmd))
	}
	if plan.OpenAndon {
		title := fmt.Sprintf("andon: promotion stop PR #%s %s %s", pr, failed.ID, step)
		body := fmt.Sprintf("Jidoka andon (ADR 0026).\n\n- PR: %s\n- Issue ID: %s\n- Failing step: %s\n- Tree root: %s\n",
			pr, failed.ID, step, plan.TreeRoot)
		created, createErr := create(cmd.Context(), owner, repoName, title, body, nil)
		if createErr != nil {
			return fmt.Errorf("open andon issue: %w", createErr)
		}
		andonURL = created.URL
	} else if plan.OpenDogfood {
		title := fmt.Sprintf("dogfood: repeated jidoka %s PR #%s %s", kind, pr, failed.ID)
		body := fmt.Sprintf("## Dogfood Finding\n\nRepeated jidoka failure kind `%s`.\n\n- PR: %s\n- Issue ID: %s\n- Failing step: %s\n- Tree root: %s\n",
			kind, pr, failed.ID, step, plan.TreeRoot)
		created, createErr := create(cmd.Context(), owner, repoName, title, body, []string{"dogfood-finding"})
		if createErr != nil {
			return fmt.Errorf("open dogfood finding: %w", createErr)
		}
		andonURL = created.URL
	}

	if err := ops.AppendPromotionStop(logPath, workerID, ops.PromotionStop{
		Action:   ops.PromotionStopTrip,
		PR:       pr,
		IssueID:  failed.ID,
		TreeRoot: plan.TreeRoot,
		Step:     step,
		Kind:     kind,
		AndonURL: andonURL,
		Dogfood:  dogfood,
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "jidoka: armed stop on tree %s (andon=%v dogfood=%v)\n",
		plan.TreeRoot, plan.OpenAndon, plan.OpenDogfood)
	return nil
}

func issueCreatorFromEnv(env Env) issueCreator {
	return func(ctx context.Context, owner, repo, title, body string, labels []string) (externalref.CreatedIssue, error) {
		return externalref.CreateGitHubIssue(ctx, owner, repo, title, body, labels, func(key string) (string, bool) {
			v := envLookup(env, key)
			return v, v != ""
		})
	}
}

func githubOwnerRepo(env Env) (owner, repo string) {
	if v := strings.TrimSpace(envLookup(env, "GITHUB_REPOSITORY")); v != "" {
		parts := strings.SplitN(v, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1]
		}
	}
	return "scullxbones", "armature"
}

func promotionGraphFromIssues(issues map[string]*materialize.Issue) promotion.Graph {
	g := make(promotion.Graph, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		g[id] = nodeFromMaterializeIssue(issue)
	}
	return g
}
