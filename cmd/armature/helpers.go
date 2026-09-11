package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// adapterExitError represents a hook exit code from the platform adapter.
// Exit-status-based blocking platforms return non-zero codes to signal blocking
// to the platform's process exit mechanism.
type adapterExitError struct {
	code int
}

func (e adapterExitError) Error() string {
	return fmt.Sprintf("hook blocked with exit code %d", e.code)
}

// protocolExitError means RunE already completed its wire protocol: a graph or
// doctor report, bootstrap JSON already written, or the git-hook stderr
// protocol (ADR 0020 §6–7). handleRootError must not append a Command Failure.
type protocolExitError struct {
	err  error
	code int
}

func (e protocolExitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e protocolExitError) Unwrap() error {
	return e.err
}

func skipCommandFailure(err error) error {
	if err == nil {
		return nil
	}
	return protocolExitError{err: err, code: 1}
}

type commandFailureEnvelope struct {
	Error *armerrors.CommandFailure `json:"error"`
}

// renderCommandFailure writes a Command Failure to w. json/agent emit
// {error:{code,cause,next_actions,exit_code}} on the writer (stdout at the
// port). Human is Error [CODE]: cause plus Try: lines. Not nested in AOC.
func renderCommandFailure(w io.Writer, format string, cf *armerrors.CommandFailure) {
	if cf == nil {
		return
	}
	if format == "json" || format == "agent" {
		b, err := json.Marshal(commandFailureEnvelope{Error: cf})
		if err != nil {
			fallback := armerrors.Unmapped(err)
			b, _ = json.Marshal(commandFailureEnvelope{Error: fallback}) //nolint:errcheck // fallback fields are always serializable
		}
		fmt.Fprintln(w, string(b))
		return
	}
	fmt.Fprintf(w, "Error [%s]: %s\n", cf.Code, cf.Cause)
	for _, action := range cf.NextActions {
		fmt.Fprintf(w, "Try: %s\n", action)
	}
}

// handleRootError maps a root Execute error to a Command Failure, writes it
// to stdout, optionally dumps --debug on stderr, and returns the process exit
// code. adapterExitError is the harness-hook platform integer and is not a
// Command Failure on the wire. protocolExitError is the same for reports and
// git hooks that already wrote their payload.
func handleRootError(stdout, stderr io.Writer, format string, debug bool, err error) int {
	if err == nil {
		return exitcodes.ExitSuccess.Int()
	}
	if ace, ok := errors.AsType[adapterExitError](err); ok {
		return ace.code
	}
	if pe, ok := errors.AsType[protocolExitError](err); ok {
		if pe.err != nil {
			fmt.Fprintln(stderr, pe.err.Error())
		}
		if debug {
			fmt.Fprintf(stderr, "DEBUG: %+v\n", err)
		}
		if pe.code == 0 {
			return 1
		}
		return pe.code
	}
	cf := armerrors.Unmapped(err)
	renderCommandFailure(stdout, format, cf)
	if debug {
		fmt.Fprintf(stderr, "DEBUG: %+v\n", err)
	}
	return cf.ExitCode
}

type executionState struct {
	ctx     *config.Context
	tracker ops.PendingPushTracker
}

type executionStateKey struct{}

type homeEmptyReasonKey struct{}

func homeEmptyReason(cmd *cobra.Command) string {
	if cmd == nil || cmd.Context() == nil {
		return ""
	}
	reason, _ := cmd.Context().Value(homeEmptyReasonKey{}).(string)
	return reason
}

func stateFromCmd(cmd *cobra.Command) (*executionState, error) {
	if cmd == nil {
		return nil, fmt.Errorf("command context unavailable")
	}
	raw := cmd.Context()
	if raw == nil {
		return nil, fmt.Errorf("command context unavailable")
	}
	state, _ := raw.Value(executionStateKey{}).(*executionState) //nolint:errcheck // comma-ok form; nil check follows immediately
	if state == nil || state.ctx == nil {
		return nil, fmt.Errorf("command execution state unavailable")
	}
	return state, nil
}

func mustState(cmd *cobra.Command) *executionState {
	state, err := stateFromCmd(cmd)
	if err != nil {
		panic(err)
	}
	return state
}

func currentCtx(cmd *cobra.Command) *config.Context {
	return mustState(cmd).ctx
}

// invocationRepoPath is the checkout the command was invoked against: --repo,
// or "." when the flag is unset. ResolveContext walks a linked worktree up to
// the parent repo, so callers that need HEAD, dirtiness, or a process cwd for
// *this* checkout must use this path rather than Context.RepoPath.
func invocationRepoPath(cmd *cobra.Command) string {
	if cmd == nil || cmd.Root() == nil {
		return "."
	}
	path, err := cmd.Root().PersistentFlags().GetString("repo")
	if err != nil || path == "" {
		return "."
	}
	return path
}

// stateDirFor returns the worker-specific state directory.
// In dual-branch mode, state lives at the worktree root (not inside .armature/).
func stateDirFor(ctx *config.Context, workerID string) string {
	if ctx.WorktreePath != "" {
		return filepath.Join(ctx.WorktreePath, "state", workerID)
	}
	return filepath.Join(ctx.IssuesDir, "state", workerID)
}

func resolveWorkerAndLog(ctx *config.Context) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("worker not initialized: command context unavailable")
	}
	workerID, err := worker.GetWorkerID(ctx.RepoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	ownerID := workerIdentityWithSlot(workerID)
	return ownerID, opsLogPath(ctx.IssuesDir, ownerID), nil
}

// opsLogPath returns the path to ownerID's ops log file under issuesDir. This is
// the single source of truth for where an owner's ops log lives; every writer
// and reader of that log (manual commands via resolveWorkerAndLog, the harness
// hook's heartbeat emission, etc.) must derive the path through here so they
// can never diverge onto a directory materialize/snapshot doesn't read.
func opsLogPath(issuesDir, ownerID string) string {
	return filepath.Join(issuesDir, "ops", ownerID+".log")
}

var validSlotPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// workerIdentityWithSlot appends the ARM_LOG_SLOT value (parallel dispatch mode)
// to workerID to form a slotted identity used in filesystem paths. Since
// ARM_LOG_SLOT is an environment variable and not fully within our control, it
// is validated against a safe charset here; an invalid value is treated as if
// ARM_LOG_SLOT were unset (fail-open, matching this codebase's heartbeat-adjacent
// pattern of never blocking the harness) rather than propagated into path
// construction.
func workerIdentityWithSlot(workerID string) string {
	slot := os.Getenv("ARM_LOG_SLOT")
	if slot == "" {
		return workerID
	}
	if !validSlotPattern.MatchString(slot) {
		fmt.Fprintf(os.Stderr, "warning: ARM_LOG_SLOT %q contains invalid characters, ignoring\n", slot)
		return workerID
	}
	return workerID + "~" + slot
}

func baseWorkerIdentity(workerID string) string {
	before, _, found := strings.Cut(workerID, "~")
	if found {
		return before
	}
	return workerID
}

func nowEpoch() int64 {
	return time.Now().Unix()
}

// worktreeGit returns a git client for the ops worktree, or nil when the
// command is not running in dual-branch mode.
func worktreeGit(ctx *config.Context) *adapters.Client {
	if ctx == nil || ctx.WorktreePath == "" {
		return nil
	}
	return adapters.New(ctx.WorktreePath)
}

func gitCommitter(ctx *config.Context) ops.GitCommitter {
	gc := worktreeGit(ctx)
	if gc == nil {
		return nil
	}
	return gc
}

// parseAcceptanceJSON decodes --acceptance flag JSON shared by create and amend.
func parseAcceptanceJSON(acceptanceJSON string) (json.RawMessage, error) {
	if acceptanceJSON == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(acceptanceJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid --acceptance JSON: %w", err)
	}
	return raw, nil
}

// resolveIssueID returns the --issue flag value, or args[0] when the flag is
// unset. The error text is shared by every command that accepts either form.
func resolveIssueID(flag string, args []string) (string, error) {
	if flag == "" && len(args) > 0 {
		flag = args[0]
	}
	if flag == "" {
		return "", fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
	}
	return flag, nil
}

// writeCommandResult emits json/agent as a single JSON object, otherwise the
// human line. Callers must include a trailing newline in humanFormat.
func writeCommandResult(cmd *cobra.Command, jsonValue any, humanFormat string, humanArgs ...any) {
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		data, _ := json.Marshal(jsonValue) //nolint:errcheck // result values are maps/structs of serializable fields
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), humanFormat, humanArgs...)
}

func structuredFormat(cmd *cobra.Command) bool {
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	return format == "json" || format == "agent"
}

func writeNamedEnvelope(w io.Writer, key string, items any, help []string) error {
	env, err := output.NewEnvelope(key, items, help)
	if err != nil {
		return err
	}
	return output.WriteEnvelope(w, env)
}

var unknownFlagNamePattern = regexp.MustCompile(`unknown (?:shorthand )?flag: ('[^']+'|--\S+|-\S+)`)

func failLoudFlagError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return err
	}
	cause := err.Error()
	if name := flagNameFromParseError(err); name != "" && !strings.Contains(cause, name) {
		cause = "unknown flag: " + name
	}
	valid := validFlagNames(cmd)
	if len(valid) > 0 {
		cause = cause + "; valid flags: " + strings.Join(valid, ", ")
	}
	return armerrors.Wrap(armerrors.CodeUSAGE, cause, []string{"arm --help"}, exitcodes.ExitUsageError.Int(), err)
}

func flagNameFromParseError(err error) string {
	if err == nil {
		return ""
	}
	m := unknownFlagNamePattern.FindStringSubmatch(err.Error())
	if len(m) < 2 {
		return ""
	}
	return strings.Trim(m[1], "'")
}

func validFlagNames(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	visit := func(f *pflag.Flag) {
		if f == nil || f.Hidden {
			return
		}
		add("--" + f.Name)
		if f.Shorthand != "" {
			add("-" + f.Shorthand)
		}
	}
	cmd.Flags().VisitAll(visit)
	sort.Strings(names)
	return names
}

// short truncates a fingerprint string to 8 characters for display, returning
// the string unchanged if it is already shorter than that to avoid a panic.
func short(fp string) string {
	if len(fp) < 8 {
		return fp
	}
	return fp[:8]
}

// initPushDeps returns the pending-push tracker for the current context.
// Dual-branch mode uses a file-backed counter; otherwise the tracker is a no-op.
func initPushDeps(ctx *config.Context) ops.PendingPushTracker {
	if ctx != nil && ctx.WorktreePath != "" {
		return ops.NewFilePushTracker(ctx.StateDir)
	}
	return ops.NoTracker{}
}

// appendOp appends an op to the log and, in dual-branch mode, commits it to the worktree branch.
func appendOp(ctx *config.Context, logPath string, op ops.Op) error {
	if ctx == nil {
		return fmt.Errorf("appendOp: command context unavailable")
	}
	if err := refuseIntroduction(ctx, []ops.Op{op}); err != nil {
		return err
	}
	return ops.AppendAndCommit(logPath, ctx.WorktreePath, op, gitCommitter(ctx))
}

// appendHighStakesOp appends an op, commits it (dual-branch), and attempts to push.
// Push errors are best-effort — the op is still committed locally.
// Used for claim, transition, assign, unassign — ops that must not be delayed.
func appendHighStakesOp(state *executionState, logPath string, op ops.Op) error {
	_, err := appendHighStakesOpIf(state, logPath, op, nil)
	return err
}

// appendHighStakesOpIf is appendHighStakesOp with an optional proceed callback
// that runs under the per-log append lock. wrote is false when proceed skipped
// the append; push is then skipped as well.
func appendHighStakesOpIf(state *executionState, logPath string, op ops.Op, proceed func() (bool, error)) (bool, error) {
	if state == nil || state.ctx == nil {
		return false, fmt.Errorf("appendHighStakesOp: command context unavailable")
	}
	ctx := state.ctx
	tracker := state.tracker
	if err := refuseIntroduction(ctx, []ops.Op{op}); err != nil {
		return false, err
	}
	gc := worktreeGit(ctx)
	wrote, err := ops.AppendAndCommitIf(logPath, ctx.WorktreePath, op, gc, proceed)
	if err != nil || !wrote {
		return wrote, err
	}
	// Push is best-effort: push via the git client (which handles retries) but ignore errors
	if gc != nil {
		if err := gc.Push("_armature"); err != nil {
			if rbErr := gc.FetchAndRebase("_armature"); rbErr == nil {
				gc.Push("_armature") //nolint:errcheck,gosec
			}
		}
		tracker.Reset() //nolint:errcheck,gosec
	}
	return true, nil
}

// appendLowStakesOp appends an op, increments the pending counter, and only
// pushes when the threshold is reached.
func appendLowStakesOp(state *executionState, logPath string, op ops.Op) error {
	return appendLowStakesOps(state, logPath, []ops.Op{op})
}

// appendLowStakesOps Introduction-checks the whole group before appending any
// of it, so a fan-out verb cannot land a prefix and then refuse the rest.
func appendLowStakesOps(state *executionState, logPath string, proposed []ops.Op) error {
	if state == nil || state.ctx == nil {
		return fmt.Errorf("appendLowStakesOp: command context unavailable")
	}
	if len(proposed) == 0 {
		return nil
	}
	ctx := state.ctx
	tracker := state.tracker
	if err := refuseIntroduction(ctx, proposed); err != nil {
		return err
	}
	gc := gitCommitter(ctx)
	threshold := ctx.Config.LowStakesPushThreshold
	if threshold <= 0 {
		threshold = 5
	}
	for _, op := range proposed {
		if err := ops.AppendAndCommit(logPath, ctx.WorktreePath, op, gc); err != nil {
			return err
		}
		n, err := tracker.Increment()
		if err != nil {
			return err
		}
		if n >= threshold {
			tracker.Reset() //nolint:errcheck,gosec
		}
	}
	return nil
}

func refuseIntroduction(ctx *config.Context, proposed []ops.Op) error {
	if ctx == nil {
		return fmt.Errorf("refuseIntroduction: command context unavailable")
	}
	var check []ops.Op
	for _, op := range proposed {
		if ops.AffectsValidity(op.Type) {
			check = append(check, op)
		}
	}
	if len(check) == 0 {
		return nil
	}
	// Replay in memory (no checkpoint rewrite) with the same timestamp sort
	// (creates before same-timestamp links) and rollup validate uses. File-concat
	// ApplyOp can drop a later-file create's inbound link under I3 interleaving.
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	allOps, _, err := readAllOpsFromDirWithOffsets(opsDir)
	if err != nil {
		return fmt.Errorf("load ops for introduction check: %w", err)
	}

	state, skipped, firstApplyErr := materialize.ReplayOpsTolerant(allOps)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "warning: introduction replay skipped %d op(s); first error: %v\n", skipped, firstApplyErr)
	}
	manifestData, err := adapters.ReadManifestFile(filepath.Join(ctx.IssuesDir, "sources"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	return validate.CheckIntroduction(state, check, validate.Options{
		Strict:       true,
		ManifestData: manifestData,
		Now:          nowEpoch(),
	})
}

// extractFieldsFromIssue extracts specified fields from an Issue and returns their values
// as a slice of strings in the order requested. For unknown fields, returns empty string.
// Fields are comma-separated (e.g., "status,title,outcome").
func extractFieldsFromIssue(issue *materialize.Issue, fieldList string) []string {
	if issue == nil {
		return []string{}
	}

	fields := strings.Split(fieldList, ",")
	var result []string

	for _, field := range fields {
		field = strings.TrimSpace(field)
		var value string
		switch field {
		case "id":
			value = issue.ID
		case "title":
			value = issue.Title
		case "type":
			value = issue.Type
		case "status":
			value = issue.Status
		case "parent":
			value = issue.Parent
		case "outcome":
			value = issue.Outcome
		case "scope":
			value = strings.Join(issue.Scope, ",")
		case "priority":
			value = issue.Priority
		case "assigned_worker":
			value = issue.AssignedWorker
		case "claimed_by":
			value = issue.ClaimedBy
		case "blocked_by":
			value = renderStringSlice(issue.BlockedBy)
		case "blocks":
			value = renderStringSlice(issue.Blocks)
		default:
			value = ""
		}
		result = append(result, value)
	}

	return result
}

func renderStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	rendered, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(rendered)
}

// readAllOpsFromDirWithOffsets reads all ops and returns offsets for checkpoint tracking.
// Returns ops slice and a map of log filename -> byte offset (end position).
// Validates that each op's worker ID matches its filename's worker ID.
// Logs warnings for any validation failures.
func readAllOpsFromDirWithOffsets(opsDir string) ([]ops.Op, map[string]int64, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return ops.ExtractOps(items), offsets, nil
}

// newSnapshotStore creates a snapshot.Store from the given config context.
// It wires opsDir from IssuesDir/ops and stateDir from StateDir.
func newSnapshotStore(ctx *config.Context) *snapshot.Store {
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	stateDir := ctx.StateDir
	return snapshot.NewStore(opsDir, stateDir)
}
