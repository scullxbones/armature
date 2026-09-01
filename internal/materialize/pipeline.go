package materialize

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/issueid"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/traceability"
)

type Options struct {
	// WriteStateFiles controls whether state files and checkpoints are written to disk.
	// When false, checkpoint reads are also skipped, forcing a full in-memory replay
	// (no incremental mode). Use false for read-only/diagnostic calls.
	WriteStateFiles bool
	ExcludeWorkerID string // If set, filters out ops from this worker (diagnostic mode only)
	EmitWarnings    bool   // Controls whether warnings are emitted to stderr
}

type Result struct {
	IssueCount   int
	OpsProcessed int
	FullReplay   bool
	UnhandledOps []ops.Op
	Warnings     []string
}

// toTraceabilityRefs converts the issues map into a slice of traceability.IssueRef
// without importing materialize from the traceability package (avoiding a cycle).
func toTraceabilityRefs(issues map[string]*Issue) []traceability.IssueRef {
	refs := make([]traceability.IssueRef, 0, len(issues))
	for id, issue := range issues {
		refs = append(refs, traceability.IssueRef{
			ID:                      id,
			SourceLinkCount:         len(issue.SourceLinks),
			CitationAcceptanceCount: len(issue.CitationAcceptances),
		})
	}
	return refs
}

// emitUnhandledOpsWarning emits a warning to stderr listing the unknown op types
// that were skipped during materialization.
func emitUnhandledOpsWarning(unhandledOps []ops.Op) {
	for _, warning := range formatUnhandledOpsWarnings(unhandledOps) {
		fmt.Fprint(os.Stderr, warning+"\n")
	}
}

func formatUnhandledOpsWarnings(unhandledOps []ops.Op) []string {
	if len(unhandledOps) == 0 {
		return nil
	}

	// Collect unique op types
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

func applyOps(state *State, allOps []ops.Op) ([]ops.Op, error) {
	return applyOpsWithTolerance(state, allOps, nil)
}

func missingTargetReplayID(err error) (string, bool) {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "claim: issue ") && strings.HasSuffix(msg, " not found"):
		return strings.TrimSuffix(strings.TrimPrefix(msg, "claim: issue "), " not found"), true
	case strings.HasPrefix(msg, "transition: issue ") && strings.HasSuffix(msg, " not found"):
		return strings.TrimSuffix(strings.TrimPrefix(msg, "transition: issue "), " not found"), true
	case strings.HasPrefix(msg, "link: source issue ") && strings.HasSuffix(msg, " not found"):
		return strings.TrimSuffix(strings.TrimPrefix(msg, "link: source issue "), " not found"), true
	case strings.HasPrefix(msg, "unlink: source issue ") && strings.HasSuffix(msg, " not found"):
		return strings.TrimSuffix(strings.TrimPrefix(msg, "unlink: source issue "), " not found"), true
	default:
		return "", false
	}
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

// runFullPipeline runs the full materialization pipeline.
// If emitWarnings is true, unknown-op warnings are printed to stderr.
// If writeStateFiles is false, disk-write operations are skipped.
func runFullPipeline(stateDir string, allOps []ops.Op,
	byteOffsets map[string]int64, emitWarnings bool, writeStateFiles bool) (*State, Result, error) {
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

	// Detect incremental vs full replay based on checkpoint. A checkpoint below
	// CurrentStateVersion accompanies snapshots this build cannot interpret
	// (see checkpoint.go), so it is treated exactly like no checkpoint at all:
	// discard the cached issues and replay the log cold. allOps is always the
	// complete log — callers read every op regardless of the checkpoint, which
	// only gates whether prior state is preloaded — so a forced cold replay
	// loses nothing but the preload.
	fullReplay := len(cp.ByteOffsets) == 0 || cp.StateVersion < CurrentStateVersion
	var state *State

	// For incremental replay, load prior state from issuesStateDir
	if !fullReplay {
		loadedIssues, err := LoadAllIssues(issuesStateDir)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", err)
		}
		state = NewState()
		state.Issues = loadedIssues
		// Undo cached rollup promotions before any handler runs: they are
		// derived, and handlers that branch on parent status would otherwise
		// see state a cold replay never produces. See TOPTIER-B1.
		state.RetractDerivedPromotions()
	} else {
		state = NewState()
	}

	sortOpsByTimestamp(allOps)

	unhandledOps, err := applyOps(state, allOps)
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

		readyPath := filepath.Join(stateDir, "ready.json")
		_ = adapters.WriteFile(readyPath, []byte("[]"), 0644) //nolint:errcheck // best-effort derived state; critical writes are checked
	}

	// Emit warning if any ops were unhandled
	if emitWarnings {
		emitUnhandledOpsWarning(unhandledOps)
	}

	if writeStateFiles {
		// Write checkpoint with byte offsets for next incremental replay.
		// If byteOffsets not provided, use empty map.
		offsets := byteOffsets
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		newCp := Checkpoint{ByteOffsets: offsets}
		if err := WriteCheckpoint(checkpointPath, newCp); err != nil {
			return nil, Result{}, fmt.Errorf("write checkpoint: %w", err)
		}

		cov := traceability.Compute(toTraceabilityRefs(state.Issues))
		_ = traceability.Write(filepath.Join(stateDir, "traceability.json"), cov) //nolint:errcheck // best-effort derived state; critical writes are checked
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

// runExcludeWorker replays ops excluding all ops from the given workerID.
// This is a diagnostic-only mode: state files and checkpoint are NOT updated.
// Returns the resulting State and Result.
func runExcludeWorker(allOps []ops.Op, excludeWorkerID string, emitWarnings bool) (*State, Result, error) {
	// Filter out ops from the excluded worker
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

	// Emit warning if any ops were unhandled
	if emitWarnings {
		emitUnhandledOpsWarning(unhandledOps)
	}
	warnings := formatUnhandledOpsWarnings(unhandledOps)

	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(filteredOps),
		FullReplay:   true,
		UnhandledOps: unhandledOps,
		Warnings:     warnings,
	}, nil
}

// Run is the unified entry point for materialization.
// It accepts pre-read ops and processes them according to the Options.
// If Options.ExcludeWorkerID is set, it runs in diagnostic-only mode (no disk writes).
// If Options.WriteStateFiles is false, no state files or checkpoints are written.
// If Options.EmitWarnings is false, warnings are suppressed from stderr.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID, opts.EmitWarnings)
	}
	return runFullPipeline(stateDir, allOps, byteOffsets, opts.EmitWarnings, opts.WriteStateFiles)
}

// Materialize runs the full materialization pipeline.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// issuesDir is used to resolve stateDir paths; allOps should be pre-read from the log files.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func Materialize(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (Result, error) {
	_, result, err := Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
	return result, err
}

// MaterializeAndReturnQuiet runs the full materialization pipeline without emitting
// warnings to stderr. Snapshot-backed commands use this to avoid duplicate warnings
// because they render returned warnings themselves.
func MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: false})
}

// MaterializeAndReturn runs the full materialization pipeline and returns the resulting State.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func MaterializeAndReturn(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
}

// MaterializeExcludeWorker replays ops excluding all ops from the given
// workerID. This is a diagnostic-only mode: state files and checkpoint are NOT
// updated. Returns the resulting State and Result.
// allOps should be pre-read from log files.
func MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error) {
	return Run("", allOps, nil, Options{ExcludeWorkerID: excludeWorkerID, EmitWarnings: true})
}

// opSortKey returns a secondary sort key so that at equal timestamps: creates
// sort first, note-deletes sort after note-adds (so tombstones survive
// same-second concurrent adds), and everything else sits in between.
func opSortKey(op ops.Op) int {
	switch op.Type {
	case ops.OpCreate:
		return 0
	case ops.OpNoteDelete:
		return 2
	default:
		return 1
	}
}

func sortOpsByTimestamp(allOps []ops.Op) {
	slices.SortStableFunc(allOps, func(a, b ops.Op) int {
		if n := cmp.Compare(a.Timestamp, b.Timestamp); n != 0 {
			return n
		}
		return cmp.Compare(opSortKey(a), opSortKey(b))
	})
}

// SortOpsByTimestamp is the materializer's replay order: timestamp ascending,
// creates before same-timestamp links, note-deletes last. Introduction
// projection must use this so the write door cannot drift from arm validate.
func SortOpsByTimestamp(allOps []ops.Op) {
	sortOpsByTimestamp(allOps)
}

// ApplyOpsSorted applies proposed ops in SortOpsByTimestamp order, then
// RunRollup. Callers that project a write (Introduction) and the full
// materializer share this path so they cannot disagree on parent/link
// back-edges or rollup-derived status.
func ApplyOpsSorted(state *State, proposed []ops.Op) error {
	if state == nil {
		return fmt.Errorf("ApplyOpsSorted: state is nil")
	}
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

// ReplayOpsTolerant replays historical ops in SortOpsByTimestamp order,
// skipping ApplyOp failures (backdated claim/link) and then running rollup.
// skipped is the number of apply failures; firstErr is the first of them.
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
