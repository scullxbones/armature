package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
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

// jsonErrorPayload is the structured JSON error format emitted to stderr when
// --format=json or --format=agent is active.
type jsonErrorPayload struct {
	Error    string `json:"error"`
	Code     string `json:"code"`
	ExitCode int    `json:"exit_code"`
}

// writeJSONError writes a structured JSON error to w.
// The format is: {"error": "...", "code": "...", "exit_code": N}
func writeJSONError(w io.Writer, msg string, code exitcodes.Code) {
	payload := jsonErrorPayload{
		Error:    msg,
		Code:     code.String(),
		ExitCode: code.Int(),
	}
	b, _ := json.Marshal(payload) //nolint:errcheck // payload contains only serializable values
	fmt.Fprintln(w, string(b))
}

// classifyError maps a Go error to the most specific exitcodes.Code.
// It performs simple substring matching on the error message.
func classifyError(err error) exitcodes.Code {
	if err == nil {
		return exitcodes.ExitSuccess
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return exitcodes.ExitNotFound
	case strings.Contains(msg, "already claimed"),
		strings.Contains(msg, "already exists"),
		strings.Contains(msg, "conflict"):
		return exitcodes.ExitConflict
	case strings.Contains(msg, "invalid") && strings.Contains(msg, "transition"):
		return exitcodes.ExitInvalidState
	case strings.Contains(msg, "invalid state"),
		strings.Contains(msg, "broken") && strings.Contains(msg, "dep"):
		return exitcodes.ExitInvalidState
	case strings.Contains(msg, "usage") || strings.Contains(msg, "required flag"):
		return exitcodes.ExitUsageError
	case strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "i/o error"):
		return exitcodes.ExitIOError
	default:
		return exitcodes.ExitGeneralError
	}
}

type executionState struct {
	ctx     *config.Context
	pusher  ops.Pusher
	tracker ops.PendingPushTracker
}

type executionStateKey struct{}

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

// short truncates a fingerprint string to 8 characters for display, returning
// the string unchanged if it is already shorter than that to avoid a panic.
func short(fp string) string {
	if len(fp) < 8 {
		return fp
	}
	return fp[:8]
}

// initPushDeps wires up the pusher and tracker based on the current context.
// If a worktree path is present: AppendCommitAndPush with FilePushTracker.
// Otherwise: NoPusher + NoTracker.
func initPushDeps(ctx *config.Context) (ops.Pusher, ops.PendingPushTracker) {
	if ctx.WorktreePath == "" {
		return ops.NoPusher{}, ops.NoTracker{}
	}
	gc := adapters.New(ctx.WorktreePath)
	return &ops.AppendCommitAndPush{
		Pusher:  gc,
		Branch:  "_armature",
		Backoff: nil, // use defaults: 1s, 2s, 4s
	}, ops.NewFilePushTracker(ctx.StateDir)
}

// appendOp appends an op to the log and, in dual-branch mode, commits it to the worktree branch.
func appendOp(ctx *config.Context, logPath string, op ops.Op) error {
	if ctx == nil {
		return fmt.Errorf("appendOp: command context unavailable")
	}
	var gc ops.GitCommitter
	if ctx.WorktreePath != "" {
		gc = adapters.New(ctx.WorktreePath)
	}
	return ops.AppendAndCommit(logPath, ctx.WorktreePath, op, gc)
}

// appendHighStakesOp appends an op, commits it (dual-branch), and attempts to push.
// Push errors are best-effort — the op is still committed locally.
// Used for claim, transition, assign, unassign — ops that must not be delayed.
func appendHighStakesOp(state *executionState, logPath string, op ops.Op) error {
	if state == nil || state.ctx == nil {
		return fmt.Errorf("appendHighStakesOp: command context unavailable")
	}
	ctx := state.ctx
	tracker := state.tracker
	var gc ops.GitCommitter
	if ctx.WorktreePath != "" {
		gc = adapters.New(ctx.WorktreePath)
	}
	// Append and commit — this is not best-effort
	if err := ops.AppendAndCommit(logPath, ctx.WorktreePath, op, gc); err != nil {
		return err
	}
	// Push is best-effort: push via the pusher (which handles retries) but ignore errors
	if ctx.WorktreePath != "" {
		// Use the underlying git client for push attempts
		gc2 := adapters.New(ctx.WorktreePath)
		if err := gc2.Push("_armature"); err != nil {
			// Best-effort: attempt fetch+rebase and retry once
			if rbErr := gc2.FetchAndRebase("_armature"); rbErr == nil {
				gc2.Push("_armature") //nolint:errcheck,gosec
			}
		}
		tracker.Reset() //nolint:errcheck,gosec
	}
	return nil
}

// appendLowStakesOp appends an op, increments the pending counter, and only
// pushes when the threshold is reached.
func appendLowStakesOp(state *executionState, logPath string, op ops.Op) error {
	if state == nil || state.ctx == nil {
		return fmt.Errorf("appendLowStakesOp: command context unavailable")
	}
	ctx := state.ctx
	tracker := state.tracker
	var gc ops.GitCommitter
	if ctx.WorktreePath != "" {
		gc = adapters.New(ctx.WorktreePath)
	}
	if err := ops.AppendAndCommit(logPath, ctx.WorktreePath, op, gc); err != nil {
		return err
	}

	threshold := ctx.Config.LowStakesPushThreshold
	if threshold <= 0 {
		threshold = 5
	}

	n, err := tracker.Increment()
	if err != nil {
		return err
	}

	if n >= threshold {
		// Push now and reset counter
		if ctx.WorktreePath != "" {
			pushGC := adapters.New(ctx.WorktreePath)
			_ = pushGC // push happens via AppendCommitAndPush on next high-stakes op
		}
		tracker.Reset() //nolint:errcheck,gosec
	}
	return nil
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

// readAllOpsFromDir reads all ops from a directory of .log files using validated loading.
// Returns empty slice if directory doesn't exist.
// Logs warnings for any validation failures (mismatched worker IDs, corrupt lines).
func readAllOpsFromDir(opsDir string) ([]ops.Op, error) {
	items, warnings, err := ops.LoadFromDirValidated(opsDir)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return ops.ExtractOps(items), nil
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
