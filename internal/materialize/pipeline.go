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

func swallowErr(err error) { _ = err }

type Options struct {
	WriteStateFiles bool
	ExcludeWorkerID string
	EmitWarnings    bool
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

func emitUnhandledOpsWarning(unhandledOps []ops.Op) {
	for _, warning := range formatUnhandledOpsWarnings(unhandledOps) {
		fmt.Fprint(os.Stderr, warning+"\n")
	}
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
	msg := err.Error()
	const suffix = " not found"
	for _, prefix := range []string{
		"claim: issue ",
		"transition: issue ",
		"link: source issue ",
		"unlink: source issue ",
	} {
		if strings.HasPrefix(msg, prefix) && strings.HasSuffix(msg, suffix) {
			return strings.TrimSuffix(strings.TrimPrefix(msg, prefix), suffix), true
		}
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

	fullReplay := len(cp.ByteOffsets) == 0 || cp.StateVersion != CurrentStateVersion
	var state *State

	if !fullReplay {
		loadedIssues, err := LoadAllIssues(issuesStateDir)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", err)
		}
		state = NewState()
		state.Issues = loadedIssues
		state.RetractDerivedPromotions()
	} else {
		state = NewState()
	}

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

	if emitWarnings {
		emitUnhandledOpsWarning(unhandledOps)
	}

	if writeStateFiles {
		offsets := byteOffsets
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		newCp := Checkpoint{ByteOffsets: offsets}
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

func runExcludeWorker(allOps []ops.Op, excludeWorkerID string, emitWarnings bool) (*State, Result, error) {
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

func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID, opts.EmitWarnings)
	}
	return runFullPipeline(stateDir, allOps, byteOffsets, opts.EmitWarnings, opts.WriteStateFiles)
}

func Materialize(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (Result, error) {
	_, result, err := Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
	return result, err
}

func MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: false})
}

func MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error) {
	return Run("", allOps, nil, Options{ExcludeWorkerID: excludeWorkerID, EmitWarnings: true})
}

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
