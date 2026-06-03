package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/workerruntime"
	"github.com/spf13/cobra"
)

type workerRuntimeDeps struct {
	state    *executionState
	workerID string
	logPath  string
	dryRun   bool
}

var runtimeIssueStateLoader = loadRuntimeIssueState

var newWorkerRuntime = func(deps *workerRuntimeDeps) *workerruntime.Runtime {
	return &workerruntime.Runtime{
		Ready: &repoReadyProvider{ctx: deps.state.ctx, workerID: deps.workerID, logicalWorkerID: baseWorkerIdentity(deps.workerID)},
		Claim: &repoClaimer{state: deps.state, workerID: deps.workerID, logPath: deps.logPath, dryRun: deps.dryRun},
		Exec:  &repoOrchestrator{ctx: deps.state.ctx, workerID: deps.workerID, logPath: deps.logPath, dryRun: deps.dryRun},
	}
}

type idleDiagnosticsProvider interface {
	IdleDiagnostics() map[string]any
}

type repoReadyProvider struct {
	ctx             *config.Context
	workerID        string
	logicalWorkerID string
	diagnostics     map[string]any
}

func (r *repoReadyProvider) NextReady(context.Context) (string, bool, error) {
	index, issues, err := runtimeIssueStateLoader(r.ctx)
	if err != nil {
		return "", false, err
	}
	entries := ready.ComputeReady(index, issues, r.logicalWorkerID)
	if len(entries) == 0 {
		notReady := ready.ExplainNotReady(index, issues)
		diag := map[string]any{
			"reason": "no_ready_work",
			"hint":   "run `arm ready --explain --format json` to inspect blocking gates",
		}
		if len(notReady) > 0 {
			ids := make([]string, 0, len(notReady))
			for id := range notReady {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			diag["blocked_count"] = len(ids)
			diag["blocked_preview"] = formatBlockedPreview(notReady, ids)
		}
		r.diagnostics = diag
		return "", false, nil
	}
	for _, entry := range entries {
		if entry.RequiresConfirmation {
			continue
		}
		if hasActiveScopeOverlap(entry.Issue, entry.Scope, index, issues, nowEpoch()) {
			continue
		}
		r.diagnostics = nil
		return entry.Issue, true, nil
	}
	reason := "requires_confirmation"
	hint := "confirm inferred/imported work before running the worker runtime"
	for _, entry := range entries {
		if hasActiveScopeOverlap(entry.Issue, entry.Scope, index, issues, nowEpoch()) {
			reason = "scope_conflict"
			hint = "resolve conflicting claimed/in-progress scope before running this worker lane"
			break
		}
	}
	r.diagnostics = map[string]any{
		"reason": reason,
		"hint":   hint,
	}
	return "", false, nil
}

func (r *repoReadyProvider) IdleDiagnostics() map[string]any { return r.diagnostics }

type repoClaimer struct {
	state    *executionState
	workerID string
	logPath  string
	dryRun   bool
}

func (c *repoClaimer) Claim(_ context.Context, issueID string) (bool, error) {
	if c.dryRun {
		return true, nil
	}
	ttl := c.state.ctx.Config.DefaultTTL
	if ttl <= 0 {
		ttl = 60
	}
	op := ops.Op{Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch(), WorkerID: c.workerID, Payload: ops.Payload{TTL: ttl}}
	if err := appendHighStakesOp(c.state, c.logPath, op); err != nil {
		return false, err
	}
	_, issues, err := runtimeIssueStateLoader(c.state.ctx)
	if err != nil {
		return false, err
	}
	issue := issues[issueID]
	if issue == nil {
		return false, nil
	}
	return issue.ClaimedBy == c.workerID, nil
}

func (c *repoClaimer) StillClaimed(_ context.Context, issueID string) (bool, error) {
	_, issues, err := runtimeIssueStateLoader(c.state.ctx)
	if err != nil {
		return false, err
	}
	issue := issues[issueID]
	if issue == nil {
		return false, nil
	}
	return issue.ClaimedBy == c.workerID, nil
}

type repoOrchestrator struct {
	ctx      *config.Context
	workerID string
	logPath  string
	dryRun   bool
}

func (o *repoOrchestrator) Run(ctx context.Context, issueID string) error {
	issue, err := materialize.LoadIssue(filepath.Join(o.ctx.StateDir, "issues", issueID+".json"))
	if err != nil {
		return fmt.Errorf("load issue %s: %w", issueID, err)
	}
	_ = issue
	runner := orchestrate.NewRepoRunner(o.ctx, o.workerID)
	result, err := runner.Run(ctx, orchestrate.RunRequest{
		TaskID:      issueID,
		WorkerID:    o.workerID,
		Harness:     "claude",
		RetryBudget: 3,
		DryRun:      o.dryRun,
	})
	if err != nil {
		return err
	}
	if !o.dryRun {
		if result.Phase == "retrying" {
			return fmt.Errorf("%w: %s", workerruntime.ErrTaskRetrying, result.Phase)
		}
		if result.Phase != "complete" {
			return fmt.Errorf("orchestration did not complete: phase=%s", result.Phase)
		}
	}
	return nil
}

func runtimeActiveScopes(issueID string, index materialize.Index, issues map[string]*materialize.Issue, now int64) map[string][]string {
	activeScopes := make(map[string][]string)
	for id, entry := range index {
		if id == issueID {
			continue
		}
		if entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress {
			continue
		}
		if isAncestorIssue(id, issueID, issues) {
			continue
		}
		if issue := issues[id]; issue != nil {
			ttl := issue.ClaimTTL
			if ttl <= 0 {
				ttl = 60
			}
			if claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, ttl, now) {
				continue
			}
		}
		activeScopes[id] = entry.Scope
	}
	return activeScopes
}

func hasActiveScopeOverlap(issueID string, scope []string, index materialize.Index, issues map[string]*materialize.Issue, now int64) bool {
	for otherID, entry := range index {
		if otherID == issueID {
			continue
		}
		if entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress {
			continue
		}
		if isAncestorIssue(otherID, issueID, issues) {
			continue
		}
		if issue := issues[otherID]; issue != nil {
			ttl := issue.ClaimTTL
			if ttl <= 0 {
				ttl = 60
			}
			if claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, ttl, now) {
				continue
			}
		}
		if claimPkg.ScopesOverlap(scope, entry.Scope) {
			return true
		}
	}
	return false
}

func isAncestorIssue(candidateAncestorID, issueID string, issues map[string]*materialize.Issue) bool {
	for cur := issueID; cur != ""; {
		issue := issues[cur]
		if issue == nil || issue.Parent == "" {
			return false
		}
		if issue.Parent == candidateAncestorID {
			return true
		}
		cur = issue.Parent
	}
	return false
}

func newWorkerRunCmd() *cobra.Command {
	var (
		maxTasks int
		dryRun   bool
		maxRun   time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the worker runtime loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state := mustState(cmd)
			workerID, logPath, err := resolveWorkerAndLog(state.ctx)
			if err != nil {
				return err
			}
			deps := &workerRuntimeDeps{state: state, workerID: workerID, logPath: logPath, dryRun: dryRun}
			rt := newWorkerRuntime(deps)
			effectiveMaxTasks := maxTasks
			if dryRun && effectiveMaxTasks == 0 {
				effectiveMaxTasks = 1
			}
			res, err := rt.Run(cmd.Context(), workerruntime.RuntimeOptions{
				WorkerID:   workerID,
				MaxTasks:   effectiveMaxTasks,
				MaxRuntime: maxRun,
				DryRun:     dryRun,
				Policy:     workerruntime.DefaultPolicy(),
			})
			if err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				payload := map[string]any{"tasks_completed": res.TasksCompleted, "final_state": res.FinalState, "dry_run": dryRun, "max_tasks": maxTasks}
				if res.FinalState == workerruntime.StateIdle && res.TasksCompleted == 0 {
					if dp, ok := rt.Ready.(idleDiagnosticsProvider); ok {
						if idle := dp.IdleDiagnostics(); len(idle) > 0 {
							payload["idle_diagnostics"] = idle
						}
					}
				}
				data, _ := json.Marshal(payload)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "worker run: tasks_completed=%d final_state=%s dry_run=%t max_tasks=%d\n",
				res.TasksCompleted, res.FinalState, dryRun, maxTasks)
			return nil
		},
	}
	cmd.Flags().IntVar(&maxTasks, "max-tasks", 0, "maximum tasks to execute before stopping (0 = no limit)")
	cmd.Flags().DurationVar(&maxRun, "max-runtime", 20*time.Minute, "maximum runtime before escalating (0 = no timeout)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "inspect runtime behavior without task mutation")
	return cmd
}

func loadRuntimeIssueState(ctx *config.Context) (materialize.Index, map[string]*materialize.Issue, error) {
	allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(ctx.IssuesDir, "ops"))
	if err != nil {
		return nil, nil, fmt.Errorf("read ops: %w", err)
	}
	if _, err := materialize.Materialize(ctx.StateDir, allOps, ctx.Mode == "single-branch", offsets); err != nil {
		return nil, nil, fmt.Errorf("materialize: %w", err)
	}
	index, err := materialize.LoadIndex(filepath.Join(ctx.StateDir, "index.json"))
	if err != nil {
		return nil, nil, err
	}
	issues := make(map[string]*materialize.Issue, len(index))
	for id := range index {
		issue, err := materialize.LoadIssue(filepath.Join(ctx.StateDir, "issues", id+".json"))
		if err == nil {
			clone := issue
			issues[id] = &clone
		}
	}
	return index, issues, nil
}

func formatBlockedPreview(notReady map[string]string, ids []string) []string {
	if len(ids) > 3 {
		ids = ids[:3]
	}
	preview := make([]string, 0, len(ids))
	for _, id := range ids {
		preview = append(preview, fmt.Sprintf("%s: %s", id, strings.TrimSpace(notReady[id])))
	}
	return preview
}
