// Package audit loads JSONL ops logs, sorts them, and marks losing claim races.
package audit

import (
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
)

type Entry struct {
	ops.Op
	LostRace bool
}

// Filter restricts which audit entries are returned.
type Filter struct {
	IssueID  string    // if non-empty, only entries targeting this issue
	WorkerID string    // if non-empty, only entries from this worker
	Since    time.Time // if non-zero, only entries with Timestamp >= Since.Unix()
}

// Load accepts a slice of pre-loaded log content strings, parses them into ops,
// merges all ops sorted by timestamp (then worker ID for stable order), applies
// the filter, and marks any losing claim ops as LostRace.
// logContents should be a slice of JSONL log lines (one op per line).
func Load(logContents []string, f Filter) ([]Entry, error) {
	var allOps []ops.Op
	for _, line := range logContents {
		if len(line) == 0 {
			continue
		}
		op, err := ops.ParseLine([]byte(line))
		if err != nil {
			continue
		}
		allOps = append(allOps, op)
	}

	sort.SliceStable(allOps, func(i, j int) bool {
		if allOps[i].Timestamp != allOps[j].Timestamp {
			return allOps[i].Timestamp < allOps[j].Timestamp
		}
		return allOps[i].WorkerID < allOps[j].WorkerID
	})

	lostRace := identifyLostRaceClaims(allOps)

	var sinceEpoch int64
	if !f.Since.IsZero() {
		sinceEpoch = f.Since.Unix()
	}

	var result []Entry
	for _, op := range allOps {
		if f.IssueID != "" && op.TargetID != f.IssueID {
			continue
		}
		if f.WorkerID != "" && op.WorkerID != f.WorkerID {
			continue
		}
		if sinceEpoch > 0 && op.Timestamp < sinceEpoch {
			continue
		}

		e := Entry{Op: op}
		if op.Type == ops.OpClaim {
			e.LostRace = lostRace[claimKey(op)]
		}
		result = append(result, e)
	}

	return result, nil
}

func claimKey(op ops.Op) string {
	return op.TargetID + "|" + op.WorkerID
}

func identifyLostRaceClaims(allOps []ops.Op) map[string]bool {
	return claim.LostRaceClaimKeys(allOps)
}
