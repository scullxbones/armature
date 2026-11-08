package materialize

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	claimpkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/issueid"
	"github.com/scullxbones/armature/internal/issueref"
	"github.com/scullxbones/armature/internal/oporder"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/traceability"
)

func swallowErr(err error) { _ = err }

type Options struct {
	WriteStateFiles bool
	ExcludeWorkerID string
	OpsWorktree     string
}

type Result struct {
	IssueCount   int
	OpsProcessed int
	FullReplay   bool
	UnhandledOps []ops.Op
	Warnings     []string
}

func coverageRefs(issues map[string]*Issue) []issueref.IssueRef {
	refs := make([]issueref.IssueRef, 0, len(issues))
	for id, issue := range issues {
		refs = append(refs, issueref.IssueRef{
			ID:                      id,
			SourceLinkCount:         len(issue.SourceLinks),
			CitationAcceptanceCount: len(issue.CitationAcceptances),
			Confidence:              issue.Provenance.Confidence,
		})
	}
	return refs
}

func formatUnhandledOpsWarnings(unhandledOps []ops.Op) []string {
	if len(unhandledOps) == 0 {
		return nil
	}

	typeSet := make(map[string]bool)
	for _, op := range unhandledOps {
		typeSet[op.Type] = true
	}
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	slices.Sort(types)

	return []string{
		fmt.Sprintf("warning: %d op(s) with unknown types skipped: [%s]",
			len(unhandledOps), strings.Join(types, ", ")),
	}
}

func isUnknownOpTypeError(err error) bool {
	return strings.HasPrefix(err.Error(), "unknown op type: ")
}

func missingTargetReplayID(err error) (string, bool) {
	var miss missingTargetError
	if errors.As(err, &miss) {
		return miss.TargetID, true
	}
	return "", false
}

func applyOpsWithTolerance(state *State, allOps []ops.Op, toleratedMissingTargetIDs map[string]bool) ([]ops.Op, error) {
	var unhandledOps []ops.Op
	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			if isUnknownOpTypeError(err) {
				unhandledOps = append(unhandledOps, op)
				continue
			}
			if missingTargetID, ok := missingTargetReplayID(err); ok {
				if toleratedMissingTargetIDs != nil && toleratedMissingTargetIDs[missingTargetID] {
					continue
				}
			}
			return unhandledOps, err
		}
	}
	return unhandledOps, nil
}

func purgeOrphanedIssues(issuesDir string, keep map[string]*Issue) error {
	issueIDs, err := adapters.ReadIssuesDir(issuesDir)
	if err != nil {
		return fmt.Errorf("read issues dir: %w", err)
	}
	for _, issueID := range issueIDs {
		if _, ok := keep[issueID]; ok {
			continue
		}
		if err := adapters.RemoveIssueJSON(issuesDir, issueID); err != nil {
			return fmt.Errorf("remove orphaned issue %s: %w", issueID, err)
		}
	}
	return nil
}

func runFullPipeline(stateDir string, allOps []ops.Op,
	byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.OpsWorktree != "" {
		if _, err := os.Stat(filepath.Join(opts.OpsWorktree, ".git")); err == nil {
			return runCommitIncremental(stateDir, allOps, byteOffsets, opts)
		}
	}
	// No git checkout at OpsWorktree: file-concat (tests and non-git layouts).
	// Keep OpsWorktree so incremental replay can still use worktreeOpsAfterOffsets
	// against the on-disk ops tree; lastCommitWorktree stays empty (no HEAD).
	// Production Load always supplies a worktree with .git and never reaches here.
	return runFileConcat(stateDir, allOps, byteOffsets, opts, "")
}

func runFileConcat(stateDir string, allOps []ops.Op,
	byteOffsets map[string]int64, opts Options, lastCommitWorktree string) (*State, Result, error) {
	issuesStateDir, cp, err := prepareWriteState(stateDir, opts.WriteStateFiles)
	if err != nil {
		return nil, Result{}, err
	}

	fullReplay := len(cp.ByteOffsets) == 0 || cp.StateVersion != CurrentStateVersion
	state, err := loadOrNewState(issuesStateDir, fullReplay)
	if err != nil {
		return nil, Result{}, err
	}

	state.RetractDerivedPromotions()
	applyOps := allOps
	if !fullReplay && opts.OpsWorktree != "" {
		delta, deltaErr := worktreeOpsAfterOffsets(opts.OpsWorktree, cp.ByteOffsets)
		if deltaErr != nil {
			return nil, Result{}, deltaErr
		}
		applyOps = delta
	}
	sortOpsByTimestamp(applyOps)

	unhandledOps, err := applyOpsWithTolerance(state, applyOps, nil)
	if err != nil {
		return nil, Result{}, err
	}

	state.RunRollup()

	if opts.WriteStateFiles {
		offsets := byteOffsets
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		newCp := Checkpoint{ByteOffsets: offsets}
		if lastCommitWorktree != "" {
			head, headErr := adapters.New(lastCommitWorktree).HeadSHA()
			if headErr != nil {
				return nil, Result{}, fmt.Errorf("ops HEAD: %w", headErr)
			}
			newCp.LastCommitSHA = head
		}
		if err := persistReplayArtifacts(stateDir, state, fullReplay, newCp); err != nil {
			return nil, Result{}, err
		}
	}

	warnings := formatUnhandledOpsWarnings(unhandledOps)
	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(applyOps),
		FullReplay:   fullReplay,
		UnhandledOps: unhandledOps,
		Warnings:     warnings,
	}, nil
}

func runExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error) {
	var filteredOps []ops.Op
	toleratedMissingTargetIDs := make(map[string]bool)
	for _, op := range allOps {
		if op.WorkerID != excludeWorkerID {
			filteredOps = append(filteredOps, op)
		} else {
			toleratedMissingTargetIDs[op.TargetID] = true
		}
	}

	sortOpsByTimestamp(filteredOps)

	state := NewState()

	unhandledOps, err := applyOpsWithTolerance(state, filteredOps, toleratedMissingTargetIDs)
	if err != nil {
		return nil, Result{}, err
	}

	state.RunRollup()

	warnings := formatUnhandledOpsWarnings(unhandledOps)

	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(filteredOps),
		FullReplay:   true,
		UnhandledOps: unhandledOps,
		Warnings:     warnings,
	}, nil
}

func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID)
	}
	return runFullPipeline(stateDir, allOps, byteOffsets, opts)
}

func loadOrNewState(issuesStateDir string, fullReplay bool) (*State, error) {
	state := NewState()
	if fullReplay {
		return state, nil
	}
	loadedIssues, err := LoadAllIssues(issuesStateDir)
	if err != nil {
		return nil, fmt.Errorf("load prior state: %w", err)
	}
	state.Issues = loadedIssues
	return state, nil
}

func sortOpsByTimestamp(allOps []ops.Op) {
	claimpkg.SortForReplay(allOps)
}

func ApplyOpsSorted(state *State, proposed []ops.Op) error {
	if state == nil {
		return fmt.Errorf("ApplyOpsSorted: state is nil")
	}
	state.RetractDerivedPromotions()
	ordered := append([]ops.Op(nil), proposed...)
	sortOpsByTimestamp(ordered)
	for _, op := range ordered {
		if err := state.ApplyOp(op); err != nil {
			return fmt.Errorf("%s %s: %w", op.Type, op.TargetID, err)
		}
	}
	state.RunRollup()
	return nil
}

func runCommitIncremental(stateDir string, _ []ops.Op, _ map[string]int64, opts Options) (*State, Result, error) {
	_, cp, err := prepareWriteState(stateDir, opts.WriteStateFiles)
	if err != nil {
		return nil, Result{}, err
	}
	fullReplay := cp.LastCommitSHA == "" || cp.StateVersion != CurrentStateVersion
	if !fullReplay {
		located, err := oporder.LocateOps(oporder.LocateInput{
			OpsWorktree:       opts.OpsWorktree,
			FromCommit:        cp.LastCommitSHA,
			ExtraPublishedTip: "HEAD",
		})
		switch {
		case err != nil && oporder.IsFromCommitMissing(err):
			// Fall through to a cold committed-order walk.
		case err != nil:
			return nil, Result{}, fmt.Errorf("locate ops: %w", err)
		default:
			return replayFromCheckpoint(stateDir, opts, cp, located)
		}
	}
	// OpsWorktree with .git always uses committed order. Never fall through to
	// timestamp-sorted runFileConcat: clock-skewed HostRefs (and any other
	// order-sensitive replay) would diverge from cold LocateOps.
	located, err := oporder.LocateOps(oporder.LocateInput{
		OpsWorktree:       opts.OpsWorktree,
		ExtraPublishedTip: "HEAD",
	})
	if err != nil {
		return nil, Result{}, fmt.Errorf("locate ops: %w", err)
	}
	return applyCommitLocated(stateDir, opts, Checkpoint{}, located, true)
}

func sortLocatedByCommit(located []oporder.LocatedOp) {
	slices.SortStableFunc(located, func(a, b oporder.LocatedOp) int {
		if n := cmp.Compare(a.Seq.CommitN, b.Seq.CommitN); n != 0 {
			return n
		}
		return cmp.Compare(a.Seq.Line, b.Seq.Line)
	})
}

// mergeCommittedWithPending builds the replay stream for a LocateOps materialize.
// Creates come first so claim-before-create cannot arise when a create is still
// unpublished. Committed creates precede pending creates so a committed parent
// exists before an unpublished child (Children links, --parent ready filters).
// Remaining committed ops keep LocateOps order (clock-skew HostRefs). Remaining
// pending ops are inserted by timestamp so an unpublished stale foreign claim
// cannot land after a later committed release and resurrect the lease on full
// rebuild.
func mergeCommittedWithPending(committed, pending []ops.Op) []ops.Op {
	sortOpsByTimestamp(pending)
	var pendingCreates, pendingRest []ops.Op
	for _, op := range pending {
		if op.Type == ops.OpCreate {
			pendingCreates = append(pendingCreates, op)
			continue
		}
		pendingRest = append(pendingRest, op)
	}
	var committedCreates, committedRest []ops.Op
	for _, op := range committed {
		if op.Type == ops.OpCreate {
			committedCreates = append(committedCreates, op)
			continue
		}
		committedRest = append(committedRest, op)
	}
	out := make([]ops.Op, 0, len(committed)+len(pending))
	out = append(out, committedCreates...)
	out = append(out, pendingCreates...)
	pi := 0
	for _, op := range committedRest {
		for pi < len(pendingRest) && pendingRest[pi].Timestamp < op.Timestamp {
			out = append(out, pendingRest[pi])
			pi++
		}
		out = append(out, op)
	}
	return append(out, pendingRest[pi:]...)
}

func applyCommitLocated(stateDir string, opts Options, cp Checkpoint, located []oporder.LocatedOp, fullReplay bool) (*State, Result, error) {
	sortLocatedByCommit(located)
	state, loadErr := loadOrNewState(filepath.Join(stateDir, "issues"), fullReplay)
	if loadErr != nil {
		return nil, Result{}, loadErr
	}
	state.RetractDerivedPromotions()
	offsetHint := cp.ByteOffsets
	if fullReplay {
		offsetHint = nil
	} else {
		var skipErr error
		located, skipErr = skipLocatedCoveredByOffsets(opts.OpsWorktree, located, offsetHint)
		if skipErr != nil {
			return nil, Result{}, skipErr
		}
	}
	committed := oporder.Ops(located)
	pending, pendErr := uncommittedWorktreeOps(opts.OpsWorktree, offsetHint)
	if pendErr != nil {
		return nil, Result{}, pendErr
	}
	newOps := mergeCommittedWithPending(committed, pending)
	unhandledOps, err := applyOpsWithTolerance(state, newOps, nil)
	if err != nil {
		return nil, Result{}, err
	}
	state.RunRollup()
	head, headErr := adapters.New(opts.OpsWorktree).HeadSHA()
	if headErr != nil {
		return nil, Result{}, fmt.Errorf("ops HEAD: %w", headErr)
	}
	if opts.WriteStateFiles {
		offsets := diskByteOffsets(opts.OpsWorktree)
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		if err := persistReplayArtifacts(stateDir, state, fullReplay, Checkpoint{
			LastCommitSHA: head,
			ByteOffsets:   offsets,
		}); err != nil {
			return nil, Result{}, err
		}
	}
	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(newOps),
		FullReplay:   fullReplay,
		UnhandledOps: unhandledOps,
		Warnings:     formatUnhandledOpsWarnings(unhandledOps),
	}, nil
}

func prepareWriteState(stateDir string, write bool) (issuesStateDir string, cp Checkpoint, err error) {
	issuesStateDir = filepath.Join(stateDir, "issues")
	if !write {
		return issuesStateDir, Checkpoint{}, nil
	}
	if err := adapters.MkdirAll(issuesStateDir, 0755); err != nil {
		return "", Checkpoint{}, fmt.Errorf("create state dir: %w", err)
	}
	cp, err = LoadCheckpoint(filepath.Join(stateDir, "checkpoint.json"))
	if err != nil {
		return "", Checkpoint{}, fmt.Errorf("load checkpoint: %w", err)
	}
	return issuesStateDir, cp, nil
}

func persistReplayArtifacts(stateDir string, state *State, fullReplay bool, cp Checkpoint) error {
	issuesStateDir := filepath.Join(stateDir, "issues")
	index := state.BuildIndex()
	if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	for _, issue := range state.Issues {
		if err := issueid.Validate(issue.ID); err != nil {
			return fmt.Errorf("validate materialized issue ID %q: %w", issue.ID, err)
		}
		if err := WriteIssue(issuesStateDir, *issue); err != nil {
			return fmt.Errorf("write issue %s: %w", issue.ID, err)
		}
	}
	if fullReplay {
		if err := purgeOrphanedIssues(issuesStateDir, state.Issues); err != nil {
			return err
		}
	}
	swallowErr(adapters.WriteFile(filepath.Join(stateDir, "ready.json"), []byte("[]"), 0644))
	if err := WriteCheckpoint(filepath.Join(stateDir, "checkpoint.json"), cp); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	cov := traceability.Compute(coverageRefs(state.Issues))
	swallowErr(traceability.Write(filepath.Join(stateDir, "traceability.json"), cov))
	return nil
}

func replayFromCheckpoint(stateDir string, opts Options, cp Checkpoint, located []oporder.LocatedOp) (*State, Result, error) {
	rebuild, err := shouldRebuildFromCommittedHistory(opts.OpsWorktree, cp, located)
	if err != nil {
		return nil, Result{}, err
	}
	if !rebuild {
		return applyCommitLocated(stateDir, opts, cp, located, false)
	}
	located, err = oporder.LocateOps(oporder.LocateInput{
		OpsWorktree:       opts.OpsWorktree,
		ExtraPublishedTip: "HEAD",
	})
	if err != nil {
		return nil, Result{}, fmt.Errorf("locate ops: %w", err)
	}
	return applyCommitLocated(stateDir, opts, Checkpoint{}, located, true)
}

// shouldRebuildFromCommittedHistory reports that cached snapshots mixed pending
// ops into LastCommitSHA state, and later work arrived — newly committed ops,
// or more pending that cold replay would place by file after the commit walk.
// Per-file byte offsets do not carry cross-worker order, so they cannot prove
// the cache is an ordered prefix of committed history. Rebuild from the full
// commit walk whenever pending-derived state meets new committed events.
func shouldRebuildFromCommittedHistory(worktree string, cp Checkpoint, located []oporder.LocatedOp) (bool, error) {
	if worktree == "" || cp.LastCommitSHA == "" {
		return false, nil
	}
	gc := adapters.New(worktree)
	pendingBeyondLast := false
	for name, off := range cp.ByteOffsets {
		if off <= 0 {
			continue
		}
		if off > int64(len(committedLogBytes(gc, cp.LastCommitSHA, name))) {
			pendingBeyondLast = true
			break
		}
	}
	if !pendingBeyondLast {
		return false, nil
	}
	if len(located) > 0 {
		return true, nil
	}
	pending, pendErr := uncommittedWorktreeOps(worktree, cp.ByteOffsets)
	if pendErr != nil {
		return false, pendErr
	}
	return len(pending) > 0, nil
}

func skipLocatedCoveredByOffsets(worktree string, located []oporder.LocatedOp, offsets map[string]int64) ([]oporder.LocatedOp, error) {
	skipLeft, err := skipLeftFromOffsets(worktree, located, offsets)
	if err != nil {
		return nil, err
	}
	if len(skipLeft) == 0 {
		return located, nil
	}
	out := make([]oporder.LocatedOp, 0, len(located))
	for _, loc := range located {
		if skipLeft[loc.Seq.Filename] > 0 {
			skipLeft[loc.Seq.Filename]--
			continue
		}
		out = append(out, loc)
	}
	return out, nil
}

func skipLeftFromOffsets(worktree string, located []oporder.LocatedOp, offsets map[string]int64) (map[string]int, error) {
	if len(located) == 0 || len(offsets) == 0 {
		return map[string]int{}, nil
	}
	gc := adapters.New(worktree)
	head, err := gc.HeadSHA()
	if err != nil {
		return nil, fmt.Errorf("ops HEAD: %w", err)
	}
	locatedN := map[string]int{}
	for _, loc := range located {
		locatedN[loc.Seq.Filename]++
	}
	skipLeft := map[string]int{}
	for name, n := range locatedN {
		applied := offsets[name]
		if applied <= 0 {
			continue
		}
		skipLeft[name] = skipCountFromCommitted(committedLogBytes(gc, head, name), applied, name, n)
	}
	return skipLeft, nil
}

func skipCountFromCommitted(committed []byte, applied int64, filename string, locatedN int) int {
	if applied >= int64(len(committed)) {
		return locatedN
	}
	n := locatedN - len(parseOwnedLogOps(committed[applied:], filename))
	if n < 0 {
		return 0
	}
	return n
}

func parseOwnedLogOps(content []byte, filename string) []ops.Op {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var out []ops.Op
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		op, err := ops.ParseLine(raw)
		if err != nil {
			continue
		}
		if filename != "" && !ops.WorkerOwnsLog(op.WorkerID, filename) {
			continue
		}
		out = append(out, op)
	}
	return out
}

func committedLogBytes(gc *adapters.Client, head, name string) []byte {
	for _, rel := range []string{"ops/" + name, ".armature/ops/" + name} {
		content, err := gc.ShowFileAtCommit(head, rel)
		if err == nil {
			return content
		}
	}
	return nil
}

func worktreeOpsAfterOffsets(worktree string, offsets map[string]int64) ([]ops.Op, error) {
	if worktree == "" {
		return nil, nil
	}
	if offsets == nil {
		offsets = map[string]int64{}
	}
	var extra []ops.Op
	for _, dir := range worktreeOpsLogDirs(worktree) {
		entries, readErr := os.ReadDir(dir.abs)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, readErr
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			start := offsets[e.Name()]
			suffix, readErr := ops.ReadLogFromOffset(filepath.Join(dir.abs, e.Name()), start)
			if readErr != nil {
				continue
			}
			for _, op := range suffix {
				if !ops.WorkerOwnsLog(op.WorkerID, e.Name()) {
					continue
				}
				extra = append(extra, op)
			}
		}
	}
	return extra, nil
}

func uncommittedWorktreeOps(worktree string, offsets map[string]int64) ([]ops.Op, error) {
	if worktree == "" {
		return nil, nil
	}
	if offsets == nil {
		offsets = map[string]int64{}
	}
	gc := adapters.New(worktree)
	head, err := gc.HeadSHA()
	if err != nil {
		return nil, fmt.Errorf("ops HEAD: %w", err)
	}
	var extra []ops.Op
	for _, dir := range worktreeOpsLogDirs(worktree) {
		entries, readErr := os.ReadDir(dir.abs)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, readErr
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			diskPath := filepath.Join(dir.abs, e.Name())
			rel := dir.rel + "/" + e.Name()
			committed, showErr := gc.ShowFileAtCommit(head, rel)
			if showErr != nil {
				alt, altErr := gc.ShowFileAtCommit(head, "ops/"+e.Name())
				if altErr == nil {
					committed = alt
				}
			}
			start := int64(len(committed))
			if off := offsets[e.Name()]; off > start {
				start = off
			}
			suffix, readErr := ops.ReadLogFromOffset(diskPath, start)
			if readErr != nil {
				continue
			}
			for _, op := range suffix {
				if !ops.WorkerOwnsLog(op.WorkerID, e.Name()) {
					continue
				}
				extra = append(extra, op)
			}
		}
	}
	return extra, nil
}

func diskByteOffsets(worktree string) map[string]int64 {
	out := make(map[string]int64)
	for _, dir := range worktreeOpsLogDirs(worktree) {
		loaded, err := ops.LoadFromDirValidated(dir.abs)
		if err != nil {
			continue
		}
		for name, off := range loaded.PhysicalEOF {
			if off > out[name] {
				out[name] = off
			}
		}
	}
	return out
}

func worktreeOpsLogDirs(worktree string) []struct{ abs, rel string } {
	return []struct{ abs, rel string }{
		{filepath.Join(worktree, "ops"), "ops"},
		{filepath.Join(worktree, ".armature", "ops"), ".armature/ops"},
	}
}

func ReplayOpsTolerant(allOps []ops.Op) (state *State, skipped int, firstErr error) {
	ordered := append([]ops.Op(nil), allOps...)
	sortOpsByTimestamp(ordered)
	state = NewState()
	for _, op := range ordered {
		if err := state.ApplyOp(op); err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	state.RunRollup()
	return state, skipped, firstErr
}
