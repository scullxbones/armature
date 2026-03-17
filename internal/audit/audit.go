package audit

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/trellis/internal/claim"
	"github.com/scullxbones/trellis/internal/ops"
)

// Entry is a single audit log entry with an optional lost-race marker.
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

// Load walks opsDir (the ops/ subdirectory of issuesDir), reads all *.log files,
// merges all ops sorted by timestamp (then worker ID for stable order), applies
// the filter, and marks any losing claim ops as LostRace.
func Load(opsDir string, f Filter) ([]Entry, error) {
	logFiles, err := filepath.Glob(filepath.Join(opsDir, "*.log"))
	if err != nil {
		return nil, err
	}

	// Collect all ops from all workers
	var allOps []ops.Op
	for _, logPath := range logFiles {
		logOps, err := ops.ReadLog(logPath)
		if err != nil {
			// Skip unreadable logs
			continue
		}
		allOps = append(allOps, logOps...)
	}

	// Sort by timestamp, then worker ID for stability
	sort.SliceStable(allOps, func(i, j int) bool {
		if allOps[i].Timestamp != allOps[j].Timestamp {
			return allOps[i].Timestamp < allOps[j].Timestamp
		}
		return allOps[i].WorkerID < allOps[j].WorkerID
	})

	// Identify lost-race claims
	lostRace := identifyLostRaceClaims(allOps)

	// Apply filter and build result
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

// claimKey returns a unique key for a claim op: targetID|workerID.
func claimKey(op ops.Op) string {
	return op.TargetID + "|" + op.WorkerID
}

// identifyLostRaceClaims groups claim ops by target issue, resolves the winner
// via claim.ResolveClaim, and returns a set of keys for the losing workers.
func identifyLostRaceClaims(allOps []ops.Op) map[string]bool {
	// Group claims by target ID
	claimsByTarget := make(map[string][]ops.Op)
	for _, op := range allOps {
		if op.Type == ops.OpClaim {
			claimsByTarget[op.TargetID] = append(claimsByTarget[op.TargetID], op)
		}
	}

	lost := make(map[string]bool)
	for _, claims := range claimsByTarget {
		if len(claims) < 2 {
			continue // no race
		}
		winner := claim.ResolveClaim(claims)
		for _, c := range claims {
			if c.WorkerID != winner.WorkerID {
				lost[claimKey(c)] = true
			}
		}
	}
	return lost
}
