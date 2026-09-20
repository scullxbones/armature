package main

import (
	"context"
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

func isTerminalStatus(status string) bool {
	return ops.IsTerminalStatus(status)
}

func swallowErr(err error) { _ = err }

func bestEffortClose(c io.Closer) {
	if c == nil {
		return
	}
	swallowErr(c.Close())
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	swallowErr(err)
	return b
}

func mustMarshalIndent(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	swallowErr(err)
	return b
}

func workerIDBestEffort(repoPath string) string {
	id, err := worker.GetWorkerID(repoPath)
	swallowErr(err)
	return id
}

type commandFailureEnvelope struct {
	Error *armerrors.CommandFailure `json:"error"`
}

func renderCommandFailure(w io.Writer, format string, cf *armerrors.CommandFailure) {
	if cf == nil {
		return
	}
	if format == "json" || format == "agent" {
		b, err := json.Marshal(commandFailureEnvelope{Error: cf})
		if err != nil {
			fallback := armerrors.Wrap(armerrors.CodeIO, err.Error(), nil, 1, err)
			b = mustMarshal(commandFailureEnvelope{Error: fallback})
		}
		fmt.Fprintln(w, string(b))
		return
	}
	fmt.Fprintf(w, "Error [%s]: %s\n", cf.Code, cf.Cause)
	for _, action := range cf.NextActions {
		fmt.Fprintf(w, "Try: %s\n", action)
	}
}

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
	cf := commandFailureAtPort(err)
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
	reason, ok := cmd.Context().Value(homeEmptyReasonKey{}).(string)
	if !ok {
		return ""
	}
	return reason
}

func shouldPrintRootHelp(format string, nonInteractive, isTTY bool) bool {
	return isTTY && !nonInteractive && format == "human"
}

func isAbsentArmatureLayout(repoPath string, resolveErr error) bool {
	if resolveErr == nil {
		return false
	}
	if !repoPathReachable(repoPath) {
		return false
	}
	_, layoutErr := config.ResolveLayout(repoPath)
	if layoutErr == nil {
		return false
	}
	return missingOpsWorktreePath(layoutErr)
}

func repoPathReachable(repoPath string) bool {
	info, err := os.Stat(repoPath)
	return err == nil && info.IsDir()
}

func missingOpsWorktreePath(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "armature.ops-worktree-path must be set") {
		return false
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "cannot change to") ||
		strings.Contains(lower, "permission denied") {
		return false
	}
	return true
}

func rejectUnknownRootArgs(cmd *cobra.Command, args []string) error {
	if versionRequested(cmd) {
		return cobra.NoArgs(cmd, args)
	}
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}

func stateFromCmd(cmd *cobra.Command) (*executionState, error) {
	if cmd == nil {
		return nil, fmt.Errorf("command context unavailable")
	}
	raw := cmd.Context()
	if raw == nil {
		return nil, fmt.Errorf("command context unavailable")
	}
	state, ok := raw.Value(executionStateKey{}).(*executionState)
	if !ok || state == nil || state.ctx == nil {
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

func attachExecutionState(cmd *cobra.Command, ctx *config.Context) {
	workerID := workerIDBestEffort(ctx.RepoPath)
	if workerID == "" {
		workerID = "default"
	}
	workerID = slottedWorkerID(workerID).String()
	ctx.StateDir = stateDirFor(ctx, workerID)
	state := &executionState{ctx: ctx, tracker: initPushDeps(ctx)}
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	cmd.SetContext(context.WithValue(baseCtx, executionStateKey{}, state))
}

func opsHistoryPrefixes(appCtx *config.Context, opsRepoPath string) (opsPrefix, legacyOpsPrefix string) {
	issuesRel := "."
	if appCtx.IssuesDir != "" && opsRepoPath != "" {
		if rel, relErr := filepath.Rel(opsRepoPath, appCtx.IssuesDir); relErr == nil {
			issuesRel = rel
		}
	}
	return filepath.Join(issuesRel, "ops"), filepath.Join(".armature", "ops")
}

func appendMergedTransitions(ctx *config.Context, logPath, workerID, intoBranch string, mergedIDs []string, stdout, stderr io.Writer) {
	for _, id := range mergedIDs {
		op := ops.Op{
			Type:      ops.OpTransition,
			TargetID:  id,
			WorkerID:  workerID,
			Timestamp: nowEpoch(),
			Payload: ops.Payload{
				To:      ops.StatusMerged,
				Outcome: "auto-detected merge into " + intoBranch,
			},
		}
		if err := appendOp(ctx, logPath, op); err != nil {
			_, _ = fmt.Fprintf(stderr, "Warning: failed to transition %s: %v\n", id, err)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "Transitioned %s to merged\n", id)
	}
}

func currentCtx(cmd *cobra.Command) *config.Context {
	return mustState(cmd).ctx
}

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
	ownerID := slottedWorkerID(workerID)
	return ownerID.String(), opsLogPath(ctx.IssuesDir, ownerID.String()), nil
}

func opsLogPath(issuesDir, ownerID string) string {
	return filepath.Join(issuesDir, "ops", ownerID+".log")
}

var validSlotPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type SlottedWorkerID string

func (id SlottedWorkerID) String() string { return string(id) }

func slottedWorkerID(workerID string) SlottedWorkerID {
	slot := os.Getenv("ARM_LOG_SLOT")
	if slot == "" {
		return SlottedWorkerID(workerID)
	}
	if !validSlotPattern.MatchString(slot) {
		fmt.Fprintf(os.Stderr, "warning: ARM_LOG_SLOT %q contains invalid characters, ignoring\n", slot)
		return SlottedWorkerID(workerID)
	}
	return SlottedWorkerID(workerID + "~" + slot)
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

func worktreeGit(ctx *config.Context) *adapters.Client {
	if ctx == nil || ctx.WorktreePath == "" {
		return nil
	}
	return adapters.New(ctx.WorktreePath)
}

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

func resolveIssueID(flag string, args []string) (string, error) {
	if flag == "" && len(args) > 0 {
		flag = args[0]
	}
	if flag == "" {
		return "", fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
	}
	return flag, nil
}

func writeCommandResult(cmd *cobra.Command, jsonValue any, humanFormat string, humanArgs ...any) {
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" || format == "agent" {
		fmt.Fprintln(cmd.OutOrStdout(), string(mustMarshal(jsonValue)))
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

func short(fp string) string {
	if len(fp) < 8 {
		return fp
	}
	return fp[:8]
}

func initPushDeps(ctx *config.Context) ops.PendingPushTracker {
	if ctx != nil && ctx.WorktreePath != "" {
		return ops.NewFilePushTracker(ctx.StateDir)
	}
	return ops.NoTracker{}
}

func appendOp(ctx *config.Context, logPath string, op ops.Op) error {
	if ctx == nil {
		return fmt.Errorf("appendOp: command context unavailable")
	}
	if err := refuseIntroduction(ctx, []ops.Op{op}); err != nil {
		return err
	}
	return ops.AppendAndCommit(logPath, ctx.WorktreePath, op, worktreeGit(ctx))
}

func appendHighStakesOp(state *executionState, logPath string, op ops.Op) error {
	_, err := appendHighStakesOpIf(state, logPath, op, nil)
	return err
}

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
	pushOpsBranchBestEffort(gc, tracker)
	return true, nil
}

func pushOpsBranchBestEffort(gc *adapters.Client, tracker ops.PendingPushTracker) {
	if gc != nil {
		if err := gc.Push("_armature"); err != nil {
			if rbErr := gc.FetchAndRebase("_armature"); rbErr == nil {
				swallowErr(gc.Push("_armature"))
			}
		}
	}
	if tracker != nil {
		swallowErr(tracker.Reset())
	}
}

func appendLowStakesOp(state *executionState, logPath string, op ops.Op) error {
	return appendLowStakesOps(state, logPath, []ops.Op{op})
}

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
	gc := worktreeGit(ctx)
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
			pushOpsBranchBestEffort(gc, tracker)
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

func newSnapshotStore(ctx *config.Context) *snapshot.Store {
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	stateDir := ctx.StateDir
	return snapshot.NewStore(opsDir, stateDir)
}

func issuesMatchingScope(index materialize.Index, pred func(string) bool) []string {
	var affected []string
	for id, entry := range index {
		for _, scopeEntry := range entry.Scope {
			if pred(scopeEntry) {
				affected = append(affected, id)
				break
			}
		}
	}
	sort.Strings(affected)
	return affected
}
