package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/deliverygate"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func newTransitionCmd() *cobra.Command {
	var issueID, to, outcome, branch, pr, fieldFlag string
	var force, skipDeliveryGate bool

	cmd := &cobra.Command{
		Use:   "transition [issue-id]",
		Short: "Transition an issue to a new status",
		Long: `Move an issue to a new status (e.g., from in-progress to done or merged).

Valid status transitions depend on the current status and workflow rules. Provide the target
status with --to (required). You may optionally record an outcome description, branch name,
or PR number to document the completion context.

When transitioning to done, you cannot be on main/master branch unless you use --force.
This enforces branch + PR discipline.`,
		Example: `  # Transition an issue to done with an outcome
  $ arm transition E6-S4-T2 --to done --outcome "Implemented all required features"

  # Transition to merged and record the PR number
  $ arm transition --issue E6-S4-T2 --to merged --pr 1234

  # Override branch check with --force
  $ arm transition E6-S4-T2 --to done --outcome "..." --force`,
		Args: func(cmd *cobra.Command, args []string) error {
			return mapTransitionError(cobra.MaximumNArgs(1)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapTransitionError(err) }()
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}
			if to == "" {
				return fmt.Errorf(`required flag(s) "to" not set`)
			}

			if !ops.ValidTransitionTargets[to] {
				valid := []string{}
				for s := range ops.ValidTransitionTargets {
					valid = append(valid, s)
				}
				sort.Strings(valid)
				return fmt.Errorf("invalid status %q: valid values are %v", to, valid)
			}
			if skipDeliveryGate && to != "done" {
				return fmt.Errorf("--skip-delivery-gate is only valid with --to done")
			}

			state := mustState(cmd)
			appCtx := state.ctx

			// Check branch discipline when transitioning to done (unless --force)
			if to == "done" && !force {
				repoPath := appCtx.RepoPath
				gc := adapters.New(repoPath)
				currentBranch, err := gc.CurrentBranch()
				if err == nil {
					// Only reject if we successfully detected we're on main/master
					if currentBranch == "main" || currentBranch == "master" {
						return fmt.Errorf("cannot transition to done while on %s branch: create a feature branch and open a PR\nUse --force to override", currentBranch)
					}
				}
				// If we can't determine the branch, allow the transition (graceful degradation)
			}

			// Warn if transitioning to done and issue has no source-link or accept-citation (unless --force)
			if to == "done" && !force {
				if uncited := isIssueUncited(issueID, appCtx); uncited {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"WARNING: issue %s has no source citation.\n"+
							"Run 'arm sources link --issue %s --source-id <UUID>' to link to a source document,\n"+
							"or 'arm sources accept-citation --issue %s --rationale \"...\"' to accept a citation.\n"+
							"Use --force to suppress this warning.\n",
						issueID, issueID, issueID)
				}
			}

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			cfg := appCtx.Config

			// Get current issue status from materialized index and load index entries for all issues
			store := newSnapshotStore(state.ctx)
			// A load error degrades gracefully to an empty index, matching the previous
			// behavior of silently ignoring missing-file errors from LoadIndex.
			index, _ := store.ReadIndex() //nolint:errcheck // missing index treated as empty; access uses ok-check
			if index == nil {
				index = make(materialize.Index)
			}
			currentStatus := ""
			var currentEntry *materialize.IndexEntry
			if entry, ok := index[issueID]; ok {
				currentStatus = entry.Status
				currentEntry = &entry
			}

			hookInput := adapters.HookInput{
				IssueID:    issueID,
				FromStatus: currentStatus,
				ToStatus:   to,
				WorkerID:   workerID,
			}
			if err := config.RunPreTransition(&cfg, hookInput); err != nil {
				return err
			}

			// Run delivery gate check when transitioning to done (unless
			// --skip-delivery-gate). This runs AFTER pre-transition hooks so it
			// validates the actual final state of the worktree: a hook (formatter,
			// code generator, test run) can modify files or leave artifacts, and
			// the gate must catch a resulting dirty tree or out-of-scope file
			// rather than checking a state that hooks are about to change.
			if to == "done" && !skipDeliveryGate {
				gateIssue, err := currentIssueFromOps(appCtx.IssuesDir, issueID)
				if err != nil {
					return fmt.Errorf("determine current issue state for delivery gate %s: %w. Use --skip-delivery-gate to bypass", issueID, err)
				}
				// The delivery gate validates a claimed issue's own worktree
				// binding, scope, and commits, so it applies to every issue
				// kind that claim actually binds to a worker's own worktree —
				// task, bug, feature, and (when actually claimed) story (see
				// internal/materialize/branch.go's worktree-branch mapping and
				// cmd/armature/claim.go's createWorktreeAndBranch, which all
				// four issue kinds go through). Epics never receive a worktree
				// at all (DeriveBranchName returns "" for them) and are
				// transitioned to done from a manually created/checked-out
				// branch outside the claimed-worktree workflow, so the gate
				// must not run for them (not even require
				// --skip-delivery-gate). Stories are dual-mode: claim.go also
				// supports claiming a story directly into its own worktree,
				// but the coordinator additionally transitions *unclaimed*
				// stories to done after the story-level auditor gate, from
				// its own checkout with no worktree binding at all — that
				// path must stay exempt too. Resolve the gate path first so
				// a cheap worktree-binding probe can tell an unclaimed story
				// apart from a claimed one before deciding whether to run
				// the full gate.
				//
				// Use the literal --repo flag value (the worker's actual
				// worktree), not appCtx.RepoPath: config resolution collapses
				// a linked worktree's path to the *main* repo root (see
				// resolveParentRepoFromWorktree in internal/config/context.go)
				// so it can locate the shared armature.ops-worktree-path
				// config -- but the delivery gate must run git
				// status/diff/log against the worker's own worktree, where
				// the actual dirty files, scoped diff, and commits live.
				gateRepoPath, _ := cmd.Flags().GetString("repo")
				if gateRepoPath == "" {
					gateRepoPath = "."
				}
				// Resolve to the worktree's top level before gating:
				// worktree.ResolveGitDir (used by VerifyIssueWorktreeBinding
				// and friends) stats "<gateRepoPath>/.git" with no walk-up,
				// so the default "." fails with "stat .git" when arm
				// transition is invoked from a subdirectory of the worktree
				// rather than its root. git itself resolves this via
				// rev-parse --show-toplevel; if that fails (e.g. not
				// actually inside a git worktree), fall back to the
				// original value so the existing checks fail closed with
				// their normal error instead of a confusing new one here.
				if resolved, resolveErr := deliverygate.ResolveWorktreeRoot(gateRepoPath); resolveErr == nil {
					gateRepoPath = resolved
				}

				runGate, resolvedGateRepoPath, gateErr := deliverygateRequired(appCtx.RepoPath, gateRepoPath, issueID, gateIssue)
				if gateErr != nil {
					return gateErr
				}
				gateRepoPath = resolvedGateRepoPath

				if runGate {
					if err := runDeliveryGateCheck(gateRepoPath, issueID, gateIssue.Type, gateIssue.ClaimedBy, gateIssue.Scope); err != nil {
						return err
					}
				}
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID,
				Payload: ops.Payload{
					To:                  to,
					Outcome:             outcome,
					Branch:              branch,
					PR:                  pr,
					SkippedDeliveryGate: skipDeliveryGate,
				},
			}
			if err := appendHighStakesOp(state, logPath, op); err != nil {
				return err
			}

			// After successful transition, check if we should warn about parent story
			if to == "done" && currentEntry != nil && currentEntry.Parent != "" {
				if err := checkAndWarnParentStoryStatus(index, issueID, cmd); err != nil {
					// Log the error but don't fail the transition
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check parent story status: %v\n", err)
				}
			}

			// If --field flag is set, extract and print only the requested field
			if fieldFlag != "" {
				// Create a minimal issue object with just the transition result
				// to extract the field from
				transitionResult := &materialize.Issue{
					ID:     issueID,
					Status: to,
				}
				fields := extractFieldsFromIssue(transitionResult, fieldFlag)
				for _, field := range fields {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), field)
				}
				return nil
			}

			writeCommandResult(cmd, map[string]string{"issue": issueID, "status": to},
				"%s → %s\n", issueID, to)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&to, "to", "", "target status")
	cmd.Flags().StringVar(&outcome, "outcome", "", "outcome description")
	cmd.Flags().StringVar(&branch, "branch", "", "feature branch name")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number")
	cmd.Flags().StringVar(&fieldFlag, "field", "", "comma-separated list of fields to extract (e.g., status)")
	cmd.Flags().BoolVar(&force, "force", false, "skip branch check when transitioning to done")
	cmd.Flags().BoolVar(&skipDeliveryGate, "skip-delivery-gate", false, "skip delivery gate check when transitioning to done")
	return cmd
}

// currentIssueFromOps reads the append-only source of truth without updating
// derived state. Delivery-gate decisions must not rely on a stale snapshot:
// amend, unassign, and transition --to open append authoritative state changes
// but do not synchronously materialize them.
func currentIssueFromOps(issuesDir, issueID string) (*materialize.Issue, error) {
	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
	if err != nil {
		return nil, fmt.Errorf("read ops: %w", err)
	}
	state, _, err := materialize.Run("", allOps, nil, materialize.Options{WriteStateFiles: false})
	if err != nil {
		return nil, fmt.Errorf("replay ops: %w", err)
	}
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue not found in current ops")
	}
	return issue, nil
}

// isIssueUncited returns true if the issue has no source-link or accept-citation.
// It reads the materialized issue directly from disk without triggering rematerialization.
// If the issue cannot be read (e.g. not yet materialized), it returns false to avoid false positives.
func isIssueUncited(issueID string, appCtx *config.Context) bool {
	store := newSnapshotStore(appCtx)
	// A read error degrades gracefully: treat the issue as not uncited, matching the previous
	// behavior of ignoring missing-file errors when the issue file was absent.
	issue, err := store.ReadIssue(issueID)
	if err != nil || issue == nil {
		// Cannot read — graceful degradation, don't warn
		return false
	}
	return len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0
}

// checkAndWarnParentStoryStatus checks if all sibling tasks will be done after this transition,
// and the parent is still in-progress. Prints a warning to stderr if so.
func checkAndWarnParentStoryStatus(index materialize.Index, currentIssueID string, cmd *cobra.Command) error {
	currentEntry, ok := index[currentIssueID]
	if !ok {
		return fmt.Errorf("current issue not found in index: %s", currentIssueID)
	}

	parentID := currentEntry.Parent
	if parentID == "" {
		return nil
	}

	parentEntry, ok := index[parentID]
	if !ok {
		return fmt.Errorf("parent issue not found: %s", parentID)
	}

	// Check if parent is still in-progress
	if parentEntry.Status != "in-progress" {
		return nil
	}

	// Check if all siblings are done or will be done after this transition.
	// We assume the current issue is transitioning to done.
	allSiblingsDone := true
	for _, siblingID := range parentEntry.Children {
		siblingEntry, ok := index[siblingID]
		if !ok {
			continue
		}
		// If this is the current issue being transitioned, assume it will be done
		if siblingID == currentIssueID {
			continue
		}
		// Otherwise, check if the sibling is already done
		if siblingEntry.Status != "done" {
			allSiblingsDone = false
			break
		}
	}

	if allSiblingsDone {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n"+
			"WARNING: All tasks under %s are done but the story is still in-progress.\n"+
			"Run: arm transition %s --to done --outcome \"...\"\n\n",
			parentID, parentID)
	}

	return nil
}

// deliverygateRequired decides whether `arm transition --to done` must run
// the delivery gate for issueID, and which worktree it should run against.
// It applies ONE rule, regardless of the issue's type or claim state: the
// gate is required whenever a live claimed-worktree binding is discoverable
// for issueID — i.e. some worktree (the invoking checkout, or any other
// worktree linked to the repo) carries an armature-issue-id marker recorded
// for issueID at claim time. gateIssue.Type and gateIssue.ClaimedBy are used
// ONLY to decide where to look for that binding, and whether its absence is
// itself an error — NEVER to skip enforcing a binding once one is found. A
// worktree marker on disk outlives both an amend that changes Type and an
// `arm unassign` that clears ClaimedBy, so neither of those mutable fields
// may be used to wave off a binding the marker proves is still live.
//
// This replaces a prior multi-branch decision tree (default-by-Type,
// story-specific worktree scan, ClaimedBy fallback) assembled incrementally
// across ~7 PR review rounds, each patching one gap the tree's branching
// opened. The tree's central flaw: it consulted current Type/ClaimedBy
// first and only fell back to checking for a live binding in the story
// case, so an issue retyped to an unmapped type after claim (e.g. task ->
// epic, done via unassign+retype) could skip the gate entirely even though
// its worktree marker, and possibly uncommitted or out-of-scope changes,
// were still live. Consolidating to "look for a binding first, let Type
// only decide where to look and whether not finding one is an error" closes
// that gap for every issue kind at once, without relaxing any individual
// check: every fail-closed error path below has a direct predecessor in the
// tree it replaces.
//
// Returns (runGate, gateRepoPath, error). gateRepoPath is the worktree the
// gate should actually run against: normally invokingRepoPath, but a
// repo-wide scan may find the live binding on a different worktree when the
// invoking checkout's own marker doesn't match (or is entirely missing) —
// see resolveClaimedStoryWorktree's doc comment. A non-nil error means the
// transition must be refused outright (fail closed), not treated as "gate
// not required".
func deliverygateRequired(repoRoot, invokingRepoPath, issueID string, gateIssue *materialize.Issue) (bool, string, error) {
	// Deliberately no epic-Type short-circuit here, even though epics
	// normally never receive a worktree (materialize.DeriveBranchName
	// returns "" for them, and `arm claim` refuses --worktree for a type
	// with no branch mapping — see claim.go). A worktree CAN still end up
	// bound to an issue that is epic RIGHT NOW: it was claimed as
	// task/bug/feature (creating the worktree+marker), then amended to epic
	// after the fact, possibly with the claim itself also released via
	// unassign. Skipping the lookup for Type == "epic" would resurrect
	// exactly the bypass this function exists to close — see
	// TestTransitionDoneUnassignedThenRetypedToEpicStillGatesLingeringWorktree.
	// A genuinely-never-claimed epic still ends up exempt below, once the
	// scan finds no binding and the "not found" fail-closed check (which
	// never fires for Type == "epic") lets it through.

	// Inspect the invoking checkout's own marker first. Three outcomes:
	//   - bound to issueID: the cheap common case (most transitions run from
	//     the worker's own claimed worktree) — gate right here.
	//   - bound to a DIFFERENT issue: this is an explicit misdirection (e.g.
	//     --repo pointed at the wrong worktree) and must fail closed against
	//     THIS checkout specifically, not be silently redirected to whatever
	//     worktree issueID actually lives in — see
	//     TestTransitionDoneRepoNotBoundToIssueFailsClosed. Route it into the
	//     gate anyway so runDeliveryGateCheck's own VerifyIssueWorktreeBinding
	//     produces that precise "bound to X, not Y" error.
	//   - unbound (no marker at all) or unresolvable: fall through to the
	//     repo-wide scan below, which is the only way to find a story (or any
	//     type) claimed into a worktree other than the one transition was
	//     invoked from.
	invokingBinding, bindingErr := worktreeIssueBinding(invokingRepoPath)
	if bindingErr == nil {
		if invokingBinding == issueID {
			return true, invokingRepoPath, nil
		}
		if invokingBinding != "" {
			return true, invokingRepoPath, nil
		}
	}

	// No (matching) binding in the invoking checkout: scan every worktree
	// linked to the repo for one whose own marker is bound to issueID,
	// regardless of what branch it currently has checked out (see
	// resolveClaimedStoryWorktree's doc comment — a claimed worktree can be
	// left in detached HEAD or on a scratch branch and still be the live
	// binding; branch-name lookup alone would miss it). A scan failure (git
	// worktree list failing, or a marker existing but unreadable for a reason
	// other than "missing") must fail the transition closed, never be
	// silently treated as "no binding found" — see armature constitution I5
	// (deterministic gates decide).
	claimedPath, found, err := resolveClaimedStoryWorktree(repoRoot, issueID)
	if err != nil {
		return false, invokingRepoPath, fmt.Errorf("could not determine claimed worktree for %s: %w. Use --skip-delivery-gate to bypass", issueID, err)
	}
	if found {
		return true, claimedPath, nil
	}

	// No live worktree binding is discoverable anywhere. Type/ClaimedBy now
	// decide only whether that absence is itself an error:
	//
	//   - task/bug/feature always go through the claimed-worktree workflow
	//     (see internal/materialize/branch.go), so a missing binding for one
	//     of these types is never a legitimate "nothing to check" — it means
	//     the worktree was removed/pruned, or the issue was never claimed at
	//     all, either of which the gate cannot validate. Fail closed exactly
	//     as it always has (previously via runDeliveryGateCheck's own
	//     VerifyIssueWorktreeBinding failing on the missing marker; now
	//     surfaced here with the same effect).
	//   - Any other still-claimed issue (story claimed directly into its own
	//     worktree, or any type amended away from task/bug/feature after
	//     claim) that has no discoverable binding likely had its worktree
	//     removed or pruned manually. Do not confuse that with a
	//     legitimately unclaimed issue: fail closed.
	if gateIssue.ClaimedBy != "" || gateIssue.Type == "task" || gateIssue.Type == "bug" || gateIssue.Type == "feature" {
		return false, invokingRepoPath, fmt.Errorf(
			"claimed issue %s has no discoverable claimed worktree; restore or re-claim it, or use --skip-delivery-gate to bypass",
			issueID)
	}

	// Never claimed (or claim fully released) and no worktree anywhere
	// carries its marker: this issue never entered, or has fully exited, the
	// bound-worktree workflow (e.g. an unclaimed coordinator-level story).
	// Stay exempt.
	return false, invokingRepoPath, nil
}

// worktreeIssueBinding reads the armature-issue-id marker file that claim
// writes into worktreePath's actual git directory at claim time (the same
// mechanism deliverygate.VerifyIssueWorktreeBinding enforces once the gate
// is known to apply). Returns ("", nil) when there is no marker (worktree
// never claimed, or not a worktree at all — mirrors
// harnesshook.ReadIssueBindingFileErr's own "missing" semantics), the bound
// issue ID when a marker exists, or a non-nil error only when resolution or
// the read itself failed for a reason other than "missing" (e.g. permission
// denied). deliverygateRequired uses the three-way result to distinguish "is
// this worktree bound to the issue I'm gating for", "is it bound to some
// OTHER issue" (an explicit misdirection that must fail closed against this
// checkout, not be silently redirected elsewhere), and "unbound" (fall
// through to a repo-wide scan for the live binding).
func worktreeIssueBinding(worktreePath string) (string, error) {
	gitDir, err := worktree.ResolveGitDir(worktreePath)
	if err != nil {
		return "", err
	}
	return harnesshook.ReadIssueBindingFileErr(gitDir)
}

// resolveClaimedStoryWorktree finds a worktree actually claimed for a story
// issue, regardless of which checkout `arm transition` is invoked from and
// regardless of what branch that worktree currently has checked out. It
// enumerates EVERY worktree linked to the repository via `git worktree list
// --porcelain` (listAllWorktreePaths) and checks each one's own
// armature-issue-id marker file directly, rather than first narrowing to
// "whichever worktree currently has refs/heads/feat/<id> checked out" via a
// branch-name lookup. Branch-name lookup is not enough: a
// claimed worktree left in a detached HEAD (mid-rebase, mid-bisect) or
// checked out to an unrelated scratch branch has no worktree whose HEAD
// matches the story branch, so a branch-first scan would miss it entirely
// and the gate would silently skip — exactly the gap described in
// docs/dogfood/findings/raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md
// and confirmed still exploitable against the branch-first version of this
// function by a follow-up review of commit 1ac1b2e5. Only the marker file,
// never the checked-out branch, decides whether a worktree is "claimed for
// issueID" here.
//
// Returns ("", false, nil) when no worktree's marker matches issueID —
// callers combine this with worktreeIssueBinding's own check of the
// invoking checkout, and both together are the only avenue for treating the
// story as unclaimed (a legitimate, ungated coordinator-level transition).
// Returns a non-nil error when the scan itself could not be trusted: `git
// worktree list` failing to execute, a worktree's git dir failing to resolve
// for a reason other than the worktree directory simply not existing
// (stale/prunable entries are skipped, not treated as errors), or a marker
// file existing but failing to read for a reason other than "missing" (e.g.
// permission denied, corrupt file) — see harnesshook.ReadIssueBindingFileErr,
// which already distinguishes "missing" (legitimately unclaimed) from "read
// failed" (must not be silently treated as unclaimed) for exactly this
// reason. Callers MUST fail the transition closed on a non-nil error rather
// than falling through to "not claimed", per the armature constitution's I5
// (deterministic gates decide, never silently skip).
func resolveClaimedStoryWorktree(repoPath, issueID string) (string, bool, error) {
	paths, err := listAllWorktreePaths(repoPath)
	if err != nil {
		return "", false, fmt.Errorf("list worktrees: %w", err)
	}
	for _, worktreePath := range paths {
		gitDir, err := worktree.ResolveGitDir(worktreePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Worktree directory no longer exists (stale/prunable
				// `git worktree list` entry) — nothing to check here.
				continue
			}
			return "", false, fmt.Errorf("resolve git dir for worktree %s: %w", worktreePath, err)
		}
		binding, err := harnesshook.ReadIssueBindingFileErr(gitDir)
		if err != nil {
			// A marker file existed but couldn't be read for a reason other
			// than "missing" — fail closed rather than silently skipping
			// this worktree as if it were unclaimed.
			return "", false, fmt.Errorf("read issue binding for worktree %s: %w", worktreePath, err)
		}
		if binding == issueID {
			return worktreePath, true, nil
		}
	}
	return "", false, nil
}

// listAllWorktreePaths returns the path of every worktree git knows about for
// the repository at repoPath (main worktree included), regardless of what
// branch each one currently has checked out. Used by
// resolveClaimedStoryWorktree, which must find a claimed worktree by its own
// armature-issue-id marker independent of branch state.
func listAllWorktreePaths(repoPath string) ([]string, error) {
	items, err := worktree.List(repoPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths, nil
}

// runDeliveryGateCheck runs the delivery gate checks when transitioning to done.
// It fails closed: if the worktree cannot be determined or the gate checks fail,
// it returns an error with per-check remediations.
func runDeliveryGateCheck(worktreePath string, issueID string, issueType string, claimedBy string, scope []string) error {
	if worktreePath == "" {
		return fmt.Errorf("no repo path available: cannot run delivery gate check. Use --skip-delivery-gate to bypass")
	}

	// Verify that worktreePath (the --repo flag value, or "." if unset) is
	// actually the worktree bound to issueID before checking it: without
	// this, a caller could point --repo at some other clean checkout while
	// the real claimed worktree for issueID is dirty or out-of-scope, and
	// the gate would pass by checking the wrong directory. Fail closed both when the
	// binding doesn't match and when no binding marker exists at all —
	// never silently allow an unbound path through.
	if err := deliverygate.VerifyIssueWorktreeBinding(worktreePath, issueID); err != nil {
		return err
	}

	// Verify the worktree's actual current branch (HEAD) is the expected
	// task branch for this issue, not merely that the armature-issue-id
	// marker file matches. The marker file persists in .git across a `git
	// checkout` of an unrelated scratch branch, so without this check a
	// worker could claim, then check out some other branch, commit clean
	// correctly-scoped changes there, and pass the gate even though the
	// actual task/<issueID> branch the coordinator will integrate never
	// received the commit.
	if err := deliverygate.VerifyIssueBranchBinding(worktreePath, issueID, issueType, claimedBy); err != nil {
		return err
	}

	// Get the base commit to scope-check against. Gating trusts ONLY the SHA
	// recorded once at claim time (deliverygate.GatedBaseCommit) — unlike
	// ResolveBaseCommit's three-tier fallback (used by non-gating callers),
	// this deliberately does NOT fall back to a dynamically recomputed or
	// default-branch-guessed base commit: either guess can silently stand in
	// for a claim record that is stale or was never made, letting the gate
	// pass against data nobody actually recorded for this claim. See
	// GatedBaseCommit's doc comment for the full rationale.
	git := adapters.New(worktreePath)
	baseCommit, err := deliverygate.GatedBaseCommit(worktreePath, issueID, git)
	if err != nil {
		// If no candidate base branch resolves, fail closed
		return fmt.Errorf("%w. Use --skip-delivery-gate to bypass", err)
	}

	// Run the delivery gate check
	result := deliverygate.DeliveryGate(worktreePath, issueID, baseCommit, scope)

	// Collect failed checks with their remediations
	failedChecks := []string{}
	if !result.CleanTree.Pass {
		failedChecks = append(failedChecks, "CleanTree: "+result.CleanTree.Remediation)
	}
	if !result.ScopeContainment.Pass {
		failedChecks = append(failedChecks, "ScopeContainment: "+result.ScopeContainment.Remediation)
	}
	if !result.CommitReference.Pass {
		failedChecks = append(failedChecks, "CommitReference: "+result.CommitReference.Remediation)
	}

	// If any check failed, return an error
	if len(failedChecks) > 0 {
		var sb strings.Builder
		sb.WriteString("delivery gate check failed:\n")
		for i, check := range failedChecks {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, check)
		}
		sb.WriteString("\nUse --skip-delivery-gate to override (audit trail will record the override)")
		return fmt.Errorf("%s", sb.String())
	}

	return nil
}

const codeTransition1 = "TRANSITION-1"

func init() {
	armerrors.Register(codeTransition1)
}

func mapTransitionError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "issue ID is required"),
		strings.Contains(msg, "required flag"),
		strings.Contains(msg, "accepts at most"),
		strings.Contains(msg, "skip-delivery-gate is only valid"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm transition --help"}, 2, err)
	case strings.Contains(msg, "invalid status"):
		return armerrors.Wrap(codeTransition1, msg, []string{"arm transition --to <valid-status>", "arm show"}, 1, err)
	case strings.Contains(msg, "cannot transition to done"),
		strings.Contains(msg, "Use --force"):
		return armerrors.Wrap(codeTransition1, msg, []string{
			"git switch task/<issue-id>",
			"arm transition --to done --force",
		}, 1, err)
	case strings.Contains(msg, "delivery gate"):
		return armerrors.Wrap(codeTransition1, msg, []string{"arm doctor", "arm show"}, 1, err)
	default:
		return armerrors.Wrap(codeTransition1, msg, []string{"arm doctor", "arm show"}, 1, err)
	}
}
