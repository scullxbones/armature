package ops

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

// OpGateEvidence records a wrapper-observed gate run on the invoking worker's log.
const OpGateEvidence = "gate-evidence"

// GateEvidence is the payload of a gate-evidence op: the process that observed
// the exit status writes the record. Dirty-tree runs set Uncommitted and are
// not citable.
type GateEvidence struct {
	Profile     string   `json:"profile"`
	Command     []string `json:"command"`
	HeadSHA     string   `json:"head_sha"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Exit        int      `json:"exit"`
	Uncommitted bool     `json:"uncommitted,omitempty"`
}

// Citable reports whether the run can satisfy a gate criterion (clean tree, exit 0).
func (e GateEvidence) Citable() bool {
	return !e.Uncommitted && e.Exit == 0
}

// AppendGateEvidence writes a gate-evidence op to the invoking worker's log.
func AppendGateEvidence(logPath, workerID string, ev GateEvidence) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal gate evidence: %w", err)
	}
	target := ev.Profile
	arr := []any{OpGateEvidence, target, ev.Start, workerID, json.RawMessage(payload)}
	line, err := json.Marshal(arr)
	if err != nil {
		return fmt.Errorf("marshal gate evidence op: %w", err)
	}
	return adapters.NewAppendLog(logPath).Append(append(line, '\n'))
}

// AppendGateEvidenceAndCommit appends evidence and, in dual-branch mode, commits the log.
func AppendGateEvidenceAndCommit(logPath, worktreePath, workerID string, ev GateEvidence, gc GitCommitter) error {
	if err := AppendGateEvidence(logPath, workerID, ev); err != nil {
		return err
	}
	if worktreePath == "" {
		return nil
	}
	relPath, err := filepath.Rel(worktreePath, logPath)
	if err != nil {
		return fmt.Errorf("resolve relative log path: %w", err)
	}
	workerPrefix := workerID
	if len(workerPrefix) > 8 {
		workerPrefix = workerPrefix[:8]
	}
	message := fmt.Sprintf("ops: %s %s by %s", OpGateEvidence, ev.Profile, workerPrefix)
	return gc.CommitWorktreeOp(relPath, message)
}

// ReadGateEvidence returns gate-evidence payloads from a single worker log.
func ReadGateEvidence(logPath string) ([]GateEvidence, error) {
	lines, err := adapters.ReadLogFromOffset(logPath, 0)
	if err != nil {
		return nil, err
	}
	var out []GateEvidence
	for _, line := range lines {
		ev, ok, err := parseGateEvidenceLine(line)
		if err != nil {
			continue
		}
		if ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

// ReadAllGateEvidence loads gate-evidence ops from every worker log under opsDir.
func ReadAllGateEvidence(opsDir string) ([]GateEvidence, error) {
	files, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		return nil, err
	}
	var out []GateEvidence
	for _, file := range files {
		evs, err := ReadGateEvidence(file)
		if err != nil {
			return nil, err
		}
		out = append(out, evs...)
	}
	return out, nil
}

func parseGateEvidenceLine(line []byte) (GateEvidence, bool, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return GateEvidence{}, false, err
	}
	if len(raw) < 5 {
		return GateEvidence{}, false, fmt.Errorf("op array must have at least 5 elements")
	}
	var opType string
	if err := json.Unmarshal(raw[0], &opType); err != nil {
		return GateEvidence{}, false, err
	}
	if opType != OpGateEvidence {
		return GateEvidence{}, false, nil
	}
	var ev GateEvidence
	if err := json.Unmarshal(raw[4], &ev); err != nil {
		return GateEvidence{}, false, fmt.Errorf("invalid gate evidence payload: %w", err)
	}
	return ev, true, nil
}
