package materialize

import (
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

func toTraceabilityRefs(issues map[string]*Issue) []traceability.IssueRef {
	refs := make([]traceability.IssueRef, 0, len(issues))
	for id, issue := range issues {
		refs = append(refs, traceability.IssueRef{
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
	return runFileConcat(stateDir, allOps, byteOffsets, opts, "")
}

func runFileConcat(stateDir string, allOps []ops.Op,
	byteOffsets map[string]int64, opts Options, lastCommitWorktree string) (*State, Result, error) {
	writeStateFiles := opts.WriteStateFiles
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")

	if writeStateFiles {
		if err := adapters.MkdirAll(issuesStateDir, 0755); err != nil {
			return nil, Result{}, fmt.Errorf("create state dir: %w", err)
		}
	}

	var cp Checkpoint
	var err error
	if writeStateFiles {
		cp, err = LoadCheckpoint(checkpointPath)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load checkpoint: %w", err)
		}
	}

	fullReplay := len(cp.ByteOffsets) == 0 || cp.StateVersion != CurrentStateVersion
	var state *State

	if !fullReplay {
		loadedIssues, err := LoadAllIssues(issuesStateDir)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", err)
		}
		state = NewState()
		state.Issues = loadedIssues
	} else {
		state = NewState()
	}

	state.RetractDerivedPromotions()
	sortOpsByTimestamp(allOps)

	unhandledOps, err := applyOpsWithTolerance(state, allOps, nil)
	if err != nil {
		return nil, Result{}, err
	}

	state.RunRollup()

	if writeStateFiles {
		index := state.BuildIndex()
		if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
			return nil, Result{}, fmt.Errorf("write index: %w", err)
		}

		for _, issue := range state.Issues {
			if err := issueid.Validate(issue.ID); err != nil {
				return nil, Result{}, fmt.Errorf("validate materialized issue ID %q: %w", issue.ID, err)
			}
			if err := WriteIssue(issuesStateDir, *issue); err != nil {
				return nil, Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
			}
		}

		if fullReplay {
			if err := purgeOrphanedIssues(issuesStateDir, state.Issues); err != nil {
				return nil, Result{}, err
			}
		}

		readyPath := filepath.Join(stateDir, "ready.json")
		swallowErr(adapters.WriteFile(readyPath, []byte("[]"), 0644))
	}

	if writeStateFiles {
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
		if err := WriteCheckpoint(checkpointPath, newCp); err != nil {
			return nil, Result{}, fmt.Errorf("write checkpoint: %w", err)
		}

		cov := traceability.Compute(toTraceabilityRefs(state.Issues))
		swallowErr(traceability.Write(filepath.Join(stateDir, "traceability.json"), cov))
	}

	warnings := formatUnhandledOpsWarnings(unhandledOps)
	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
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

func sortOpsByTimestamp(allOps []ops.Op) {
	// Pre-C0 / file-concat replay: timestamp then type key (claim-ttl).
	// Post-C0 commit order lives in oporder.SortLocated / OwnerOf.
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

func runCommitIncremental(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	if opts.WriteStateFiles {
		if err := adapters.MkdirAll(issuesStateDir, 0755); err != nil {
			return nil, Result{}, fmt.Errorf("create state dir: %w", err)
		}
	}
	var cp Checkpoint
	if opts.WriteStateFiles {
		loaded, err := LoadCheckpoint(checkpointPath)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load checkpoint: %w", err)
		}
		cp = loaded
	}
	shaMissing := false
	fullReplay := cp.LastCommitSHA == "" || cp.StateVersion != CurrentStateVersion
	if !fullReplay {
		located, err := oporder.LocateOps(oporder.LocateInput{
			OpsWorktree:       opts.OpsWorktree,
			FromCommit:        cp.LastCommitSHA,
			ExtraPublishedTip: "HEAD",
		})
		switch {
		case err != nil && oporder.IsFromCommitMissing(err):
			shaMissing = true
			fullReplay = true
		case err != nil:
			return nil, Result{}, fmt.Errorf("locate ops: %w", err)
		default:
			return applyCommitLocated(stateDir, opts, cp, located, false)
		}
	}
	if shaMissing || len(allOps) == 0 {
		located, err := oporder.LocateOps(oporder.LocateInput{
			OpsWorktree:       opts.OpsWorktree,
			ExtraPublishedTip: "HEAD",
		})
		if err != nil {
			return nil, Result{}, fmt.Errorf("locate ops: %w", err)
		}
		return applyCommitLocated(stateDir, opts, Checkpoint{}, located, true)
	}
	return runFileConcat(stateDir, allOps, byteOffsets, opts, opts.OpsWorktree)
}

func sortLocatedByCommit(located []oporder.LocatedOp) {
	slices.SortStableFunc(located, func(a, b oporder.LocatedOp) int {
		if n := cmp.Compare(a.Seq.CommitN, b.Seq.CommitN); n != 0 {
			return n
		}
		return cmp.Compare(a.Seq.Line, b.Seq.Line)
	})
}

func applyCommitLocated(stateDir string, opts Options, cp Checkpoint, located []oporder.LocatedOp, fullReplay bool) (*State, Result, error) {
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	sortLocatedByCommit(located)
	var state *State
	if !fullReplay {
		loadedIssues, loadErr := LoadAllIssues(issuesStateDir)
		if loadErr != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", loadErr)
		}
		state = NewState()
		state.Issues = loadedIssues
	} else {
		state = NewState()
	}
	state.RetractDerivedPromotions()
	newOps := oporder.Ops(located)
	offsetHint := cp.ByteOffsets
	if fullReplay {
		offsetHint = nil
	}
	pending, pendErr := uncommittedWorktreeOps(opts.OpsWorktree, offsetHint)
	if pendErr != nil {
		return nil, Result{}, pendErr
	}
	newOps = append(newOps, pending...)
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
		index := state.BuildIndex()
		if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
			return nil, Result{}, fmt.Errorf("write index: %w", err)
		}
		for _, issue := range state.Issues {
			if err := issueid.Validate(issue.ID); err != nil {
				return nil, Result{}, fmt.Errorf("validate materialized issue ID %q: %w", issue.ID, err)
			}
			if err := WriteIssue(issuesStateDir, *issue); err != nil {
				return nil, Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
			}
		}
		if fullReplay {
			if err := purgeOrphanedIssues(issuesStateDir, state.Issues); err != nil {
				return nil, Result{}, err
			}
		}
		swallowErr(adapters.WriteFile(filepath.Join(stateDir, "ready.json"), []byte("[]"), 0644))
		offsets := diskByteOffsets(opts.OpsWorktree)
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		if err := WriteCheckpoint(checkpointPath, Checkpoint{
			LastCommitSHA: head,
			ByteOffsets:   offsets,
		}); err != nil {
			return nil, Result{}, fmt.Errorf("write checkpoint: %w", err)
		}
		cov := traceability.Compute(toTraceabilityRefs(state.Issues))
		swallowErr(traceability.Write(filepath.Join(stateDir, "traceability.json"), cov))
	}
	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(newOps),
		FullReplay:   fullReplay,
		UnhandledOps: unhandledOps,
		Warnings:     formatUnhandledOpsWarnings(unhandledOps),
	}, nil
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
	dirs := []struct {
		abs string
		rel string
	}{
		{filepath.Join(worktree, "ops"), "ops"},
		{filepath.Join(worktree, ".armature", "ops"), ".armature/ops"},
	}
	for _, dir := range dirs {
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
			expectedWorkerID := strings.TrimSuffix(e.Name(), ".log")
			legacyWorkerID, _, _ := strings.Cut(expectedWorkerID, "~")
			for _, op := range suffix {
				if op.WorkerID != expectedWorkerID && op.WorkerID != legacyWorkerID {
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
	for _, dir := range []string{filepath.Join(worktree, "ops"), filepath.Join(worktree, ".armature", "ops")} {
		loaded, err := ops.LoadFromDirValidated(dir)
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
