// Package audit loads JSONL ops logs, sorts them, and marks losing claim races.
package audit

import (
	"fmt"
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

// Input is one worker log's JSONL lines, used so corrupt-line warnings can name
// the file and 1-based line number the way ops loading reports warnings.
type Input struct {
	File  string
	Lines []string
}

// Load accepts pre-loaded JSONL log lines (one op per line), parses them into
// ops, merges all ops sorted by timestamp (then worker ID for stable order),
// applies the filter, and marks any losing claim ops as LostRace.
// Corrupt lines are skipped and returned in the warning list.
func Load(logs []Input, f Filter) ([]Entry, []string, error) {
	var allOps []ops.Op
	var warnings []string
	for _, log := range logs {
		file := log.File
		if file == "" {
			file = "log"
		}
		for i, line := range log.Lines {
			if len(line) == 0 {
				continue
			}
			op, err := ops.ParseLine([]byte(line))
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("corrupt line in %s:%d: %v", file, i+1, err))
				continue
			}
			allOps = append(allOps, op)
		}
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

	return result, warnings, nil
}

func claimKey(op ops.Op) string {
	return claim.ClaimOpKey(op)
}

func identifyLostRaceClaims(allOps []ops.Op) map[string]bool {
	return claim.LostRaceClaimKeys(allOps)
}
