package ops

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/armature/internal/adapters"
)

// OpPromotionStop records a jidoka tree-scoped promotion stop (ADR 0026).
// TargetID is the tree root. Audit-only: does not mutate DAG state.
const OpPromotionStop = "promotion-stop"

const (
	// PromotionStopTrip arms a tree-scoped stop.
	PromotionStopTrip = "trip"
	// PromotionStopClear disarms a stop after a root-cause note.
	PromotionStopClear = "clear"
)

// PromotionStop is the payload of a promotion-stop op.
type PromotionStop struct {
	Action   string `json:"action"`
	PR       string `json:"pr,omitempty"`        // GitHub PR number
	IssueID  string `json:"issue_id,omitempty"`  // affected leaf / subject
	TreeRoot string `json:"tree_root,omitempty"` // issue-tree root covered
	Step     string `json:"step,omitempty"`      // failing step name
	Kind     string `json:"kind,omitempty"`      // failure kind (repeat detection)
	AndonURL string `json:"andon_url,omitempty"` // GitHub andon issue URL
	NoteID   string `json:"note_id,omitempty"`   // root-cause note on clear
	Dogfood  bool   `json:"dogfood,omitempty"`   // second same-kind → dogfood
	At       int64  `json:"at,omitempty"`        // unix seconds when recorded
	// Seq is a read-time tie-break for ActiveStops when At collides.
	// Assigned by ReadAllPromotionStops (sorted log basename, then line order).
	// Not serialized into the op payload.
	Seq int `json:"-"`
}

// AppendPromotionStop writes a promotion-stop op to the invoking worker's log.
func AppendPromotionStop(logPath, workerID string, stop PromotionStop) error {
	if stop.Action != PromotionStopTrip && stop.Action != PromotionStopClear {
		return fmt.Errorf("promotion-stop: action must be %q or %q", PromotionStopTrip, PromotionStopClear)
	}
	if stop.TreeRoot == "" {
		return fmt.Errorf("promotion-stop: tree_root is required")
	}
	if stop.At == 0 {
		return fmt.Errorf("promotion-stop: at is required")
	}
	payload, err := json.Marshal(stop)
	if err != nil {
		return fmt.Errorf("marshal promotion-stop: %w", err)
	}
	line, err := marshalVersioned(OpPromotionStop, stop.TreeRoot, stop.At, workerID, payload, 0)
	if err != nil {
		return fmt.Errorf("marshal promotion-stop op: %w", err)
	}
	return adapters.NewAppendLog(logPath).Append(append(line, '\n'))
}

// ReadAllPromotionStops loads promotion-stop ops from every worker log under opsDir.
// Stops are tagged with Seq in deterministic order: sorted log basename, then
// append order within each file. Callers that fold history (ActiveStops) must
// sort by At then Seq — never by ReadDir encounter order alone.
func ReadAllPromotionStops(opsDir string) ([]PromotionStop, error) {
	files, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return filepath.Base(files[i]) < filepath.Base(files[j])
	})
	var out []PromotionStop
	seq := 0
	for _, file := range files {
		stops, err := readPromotionStops(file)
		if err != nil {
			return nil, err
		}
		for i := range stops {
			stops[i].Seq = seq
			seq++
		}
		out = append(out, stops...)
	}
	return out, nil
}

func readPromotionStops(logPath string) ([]PromotionStop, error) {
	lines, err := adapters.ReadLogFromOffset(logPath, 0)
	if err != nil {
		return nil, err
	}
	var out []PromotionStop
	var nInvalid int
	for _, line := range lines {
		stop, ok, err := parsePromotionStopLine(line)
		if err != nil {
			nInvalid++
			continue
		}
		if ok {
			out = append(out, stop)
		}
	}
	if nInvalid > 0 {
		return nil, fmt.Errorf("discarded %d invalid promotion-stop line(s)", nInvalid)
	}
	return out, nil
}

func parsePromotionStopLine(line []byte) (PromotionStop, bool, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return PromotionStop{}, false, nil
	}
	if len(raw) < 1 {
		return PromotionStop{}, false, nil
	}
	var opType string
	if err := json.Unmarshal(raw[0], &opType); err != nil {
		return PromotionStop{}, false, nil
	}
	if opType != OpPromotionStop {
		return PromotionStop{}, false, nil
	}
	if len(raw) < 5 {
		return PromotionStop{}, false, fmt.Errorf("invalid promotion-stop payload: op array must have at least 5 elements")
	}
	var stop PromotionStop
	if err := json.Unmarshal(raw[4], &stop); err != nil {
		return PromotionStop{}, false, fmt.Errorf("invalid promotion-stop payload: %w", err)
	}
	return stop, true, nil
}
