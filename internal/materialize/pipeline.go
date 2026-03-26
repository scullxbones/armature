package materialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/traceability"
)

type Result struct {
	IssueCount   int
	OpsProcessed int
	FullReplay   bool
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

// Materialize runs the full materialization pipeline.
func Materialize(issuesDir string, singleBranch bool) (Result, error) {
	opsDir := filepath.Join(issuesDir, "ops")
	stateDir := filepath.Join(issuesDir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")

	_ = os.MkdirAll(issuesStateDir, 0755)

	cp, err := LoadCheckpoint(checkpointPath)
	if err != nil {
		return Result{}, fmt.Errorf("load checkpoint: %w", err)
	}

	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No ops dir yet — empty result
			return Result{}, nil
		}
		return Result{}, fmt.Errorf("read ops dir: %w", err)
	}

	var allOps []ops.Op
	newOffsets := make(map[string]int64)
	// Always do a full replay from offset 0 to ensure correct accumulated state.
	// Incremental reads would require loading prior state before applying new ops.
	fullReplay := true
	_ = cp // checkpoint offsets reserved for future optimization

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
		workerID := ops.WorkerIDFromFilename(logPath)

		logOps, err := ops.ReadLogFromOffset(logPath, 0)
		if err != nil {
			return Result{}, fmt.Errorf("read log %s: %w", entry.Name(), err)
		}

		for _, op := range logOps {
			if op.WorkerID != workerID {
				continue
			}
			allOps = append(allOps, op)
		}

		info, _ := os.Stat(logPath)
		if info != nil {
			newOffsets[entry.Name()] = info.Size()
		}
	}

	sortOpsByTimestamp(allOps)

	state := NewState()
	state.SingleBranchMode = singleBranch

	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			continue
		}
	}

	state.RunRollup()

	index := state.BuildIndex()
	if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
		return Result{}, fmt.Errorf("write index: %w", err)
	}

	for _, issue := range state.Issues {
		if err := WriteIssue(issuesStateDir, *issue); err != nil {
			return Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
		}
	}

	readyPath := filepath.Join(stateDir, "ready.json")
	_ = os.WriteFile(readyPath, []byte("[]"), 0644)

	if fullReplay && len(allOps) > 100 {
		fmt.Fprintf(os.Stderr, "Full replay: processed %d ops across %d issues\n", len(allOps), len(state.Issues))
	}

	newCp := Checkpoint{ByteOffsets: newOffsets}
	if err := WriteCheckpoint(checkpointPath, newCp); err != nil {
		return Result{}, fmt.Errorf("write checkpoint: %w", err)
	}

	cov := traceability.Compute(toTraceabilityRefs(state.Issues))
	_ = traceability.Write(filepath.Join(stateDir, "traceability.json"), cov)

	return Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
		FullReplay:   fullReplay,
	}, nil
}

// MaterializeAndReturn runs the full materialization pipeline and returns the resulting State.
func MaterializeAndReturn(issuesDir string, singleBranch bool) (*State, Result, error) {
	opsDir := filepath.Join(issuesDir, "ops")
	stateDir := filepath.Join(issuesDir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")

	_ = os.MkdirAll(issuesStateDir, 0755)

	cp, err := LoadCheckpoint(checkpointPath)
	if err != nil {
		return nil, Result{}, fmt.Errorf("load checkpoint: %w", err)
	}

	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), Result{}, nil
		}
		return nil, Result{}, fmt.Errorf("read ops dir: %w", err)
	}

	var allOps []ops.Op
	newOffsets := make(map[string]int64)
	// Always do a full replay from offset 0 to ensure correct accumulated state.
	fullReplay := true
	_ = cp // checkpoint offsets reserved for future optimization

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
		workerID := ops.WorkerIDFromFilename(logPath)

		logOps, err := ops.ReadLogFromOffset(logPath, 0)
		if err != nil {
			return nil, Result{}, fmt.Errorf("read log %s: %w", entry.Name(), err)
		}

		for _, op := range logOps {
			if op.WorkerID != workerID {
				continue
			}
			allOps = append(allOps, op)
		}

		info, _ := os.Stat(logPath)
		if info != nil {
			newOffsets[entry.Name()] = info.Size()
		}
	}

	sortOpsByTimestamp(allOps)

	state := NewState()
	state.SingleBranchMode = singleBranch

	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			continue
		}
	}

	state.RunRollup()

	index := state.BuildIndex()
	if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
		return nil, Result{}, fmt.Errorf("write index: %w", err)
	}

	for _, issue := range state.Issues {
		if err := WriteIssue(issuesStateDir, *issue); err != nil {
			return nil, Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
		}
	}

	readyPath := filepath.Join(stateDir, "ready.json")
	_ = os.WriteFile(readyPath, []byte("[]"), 0644)

	if fullReplay && len(allOps) > 100 {
		fmt.Fprintf(os.Stderr, "Full replay: processed %d ops across %d issues\n", len(allOps), len(state.Issues))
	}

	newCp := Checkpoint{ByteOffsets: newOffsets}
	if err := WriteCheckpoint(checkpointPath, newCp); err != nil {
		return nil, Result{}, fmt.Errorf("write checkpoint: %w", err)
	}

	cov := traceability.Compute(toTraceabilityRefs(state.Issues))
	_ = traceability.Write(filepath.Join(stateDir, "traceability.json"), cov)

	result := Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
		FullReplay:   fullReplay,
	}
	return state, result, nil
}

// MaterializeExcludeWorker replays the op log excluding all ops from the given
// workerID. This is a diagnostic-only mode: state files and checkpoint are NOT
// updated. Returns the resulting State and Result.
func MaterializeExcludeWorker(issuesDir, excludeWorkerID string, singleBranch bool) (*State, Result, error) {
	opsDir := filepath.Join(issuesDir, "ops")

	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), Result{}, nil
		}
		return nil, Result{}, fmt.Errorf("read ops dir: %w", err)
	}

	var allOps []ops.Op

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		// Skip the log file belonging to the excluded worker.
		if strings.Contains(entry.Name(), excludeWorkerID) {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
		workerID := ops.WorkerIDFromFilename(logPath)
		if workerID == excludeWorkerID {
			continue
		}

		logOps, err := ops.ReadLogFromOffset(logPath, 0)
		if err != nil {
			return nil, Result{}, fmt.Errorf("read log %s: %w", entry.Name(), err)
		}

		for _, op := range logOps {
			if op.WorkerID != workerID {
				continue
			}
			allOps = append(allOps, op)
		}
	}

	sortOpsByTimestamp(allOps)

	state := NewState()
	state.SingleBranchMode = singleBranch

	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			continue
		}
	}

	state.RunRollup()

	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
		FullReplay:   true,
	}, nil
}

// opSortKey returns a secondary sort key so that create ops are processed
// before other op types when timestamps are equal.
func opSortKey(op ops.Op) int {
	if op.Type == ops.OpCreate {
		return 0
	}
	return 1
}

func sortOpsByTimestamp(allOps []ops.Op) {
	for i := 1; i < len(allOps); i++ {
		for j := i; j > 0; j-- {
			a, b := allOps[j-1], allOps[j]
			if a.Timestamp < b.Timestamp {
				break
			}
			if a.Timestamp == b.Timestamp && opSortKey(a) <= opSortKey(b) {
				break
			}
			allOps[j], allOps[j-1] = allOps[j-1], allOps[j]
		}
	}
}
