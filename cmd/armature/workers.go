package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

// WorkerStatus describes the current activity state of a worker.
type WorkerStatus struct {
	WorkerID    string `json:"worker_id"`
	Status      string `json:"status"`       // "active", "stale", or "idle"
	LastOpTime  int64  `json:"last_op_time"` // Unix epoch of most recent op
	ActiveIssue string `json:"active_issue,omitempty"`
}

func newWorkersCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Show worker activity status",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			opsDir := filepath.Join(appCtx.IssuesDir, "ops")
			defaultTTL := appCtx.Config.DefaultTTL
			if defaultTTL <= 0 {
				defaultTTL = 60
			}
			now := time.Now().Unix()

			workers, err := enumerateWorkers(opsDir)
			if err != nil {
				return fmt.Errorf("enumerate workers: %w", err)
			}

			statuses := make([]WorkerStatus, 0, len(workers))
			winners := claimWinnersByIssue(workers)
			for workerID, allOps := range workers {
				s := buildWorkerStatus(workerID, allOps, defaultTTL, now, winners)
				statuses = append(statuses, s)
			}

			// Sort by worker ID for stable output
			sort.Slice(statuses, func(i, j int) bool {
				return statuses[i].WorkerID < statuses[j].WorkerID
			})

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if jsonOut || format == "json" || format == "agent" {
				for _, s := range statuses {
					data, _ := json.Marshal(s) //nolint:errcheck // result struct contains only serializable values
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				}
				return nil
			}

			if len(statuses) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workers found.")
				return nil
			}
			for _, s := range statuses {
				lastSeen := ""
				if s.LastOpTime > 0 {
					lastSeen = time.Unix(s.LastOpTime, 0).UTC().Format("2006-01-02T15:04:05Z")
				}
				active := ""
				if s.ActiveIssue != "" {
					active = fmt.Sprintf(" (working on %s)", s.ActiveIssue)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-40s  %-8s  %s%s\n",
					s.WorkerID, s.Status, lastSeen, active)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSONL")
	return cmd
}

// enumerateWorkers reads all *.log files from opsDir and returns a map of
// workerID -> their ops.
func enumerateWorkers(opsDir string) (map[string][]ops.Op, error) {
	logFiles, err := filepath.Glob(filepath.Join(opsDir, "*.log"))
	if err != nil {
		return nil, err
	}

	result := make(map[string][]ops.Op)
	for _, logPath := range logFiles {
		workerID := adapters.WorkerIDFromFilename(logPath)
		logOps, err := ops.ReadLog(logPath)
		if err != nil {
			continue
		}
		result[workerID] = append(result[workerID], logOps...)
	}
	return result, nil
}

// buildWorkerStatus determines the status of a worker based on their ops:
//   - active: has a live (non-stale) claim
//   - stale: had claims but all are stale
//   - idle: last op was within 2*defaultTTL minutes window (no active claim)
func buildWorkerStatus(workerID string, allOps []ops.Op, defaultTTLMinutes int, now int64, winners map[string]string) WorkerStatus {
	lastOp := lastOpTimestampFromLog(allOps)

	// Find active claims: look for claims not yet overtaken by a transition to done/merged
	// Track claimed issues and their last state
	claimedAt := make(map[string]int64)
	lastHeartbeat := make(map[string]int64)
	claimTTL := make(map[string]int)
	transitioned := make(map[string]bool)
	claimedBy := make(map[string]string)
	lastClaimingWorkerActivity := make(map[string]int64)

	for _, op := range allOps {
		switch op.Type {
		case ops.OpClaim:
			claimedAt[op.TargetID] = op.Timestamp
			claimTTL[op.TargetID] = op.Payload.TTL
			claimedBy[op.TargetID] = op.WorkerID
			lastClaimingWorkerActivity[op.TargetID] = op.Timestamp
		case ops.OpHeartbeat:
			if op.Timestamp > lastHeartbeat[op.TargetID] {
				lastHeartbeat[op.TargetID] = op.Timestamp
			}
		case ops.OpTransition:
			if op.Payload.To == ops.StatusDone || op.Payload.To == ops.StatusMerged ||
				op.Payload.To == ops.StatusCancelled {
				transitioned[op.TargetID] = true
			}
			// Only count a transition as claiming-worker activity when the
			// transition's author is the current claim owner for this issue —
			// one worker's transition must not extend another worker's claim.
			if op.WorkerID == claimedBy[op.TargetID] && op.Timestamp > lastClaimingWorkerActivity[op.TargetID] {
				lastClaimingWorkerActivity[op.TargetID] = op.Timestamp
			}
		}
	}

	// Check each claimed issue
	for issueID, ca := range claimedAt {
		if winner, ok := winners[issueID]; ok && baseWorkerIdentity(winner) != workerID {
			continue
		}
		if transitioned[issueID] {
			continue
		}
		ttl := claimTTL[issueID]
		if ttl <= 0 {
			ttl = defaultTTLMinutes
		}
		if !claim.IsClaimStale(ca, lastHeartbeat[issueID], lastClaimingWorkerActivity[issueID], ttl, now) {
			return WorkerStatus{
				WorkerID:    workerID,
				Status:      "active",
				LastOpTime:  lastOp,
				ActiveIssue: issueID,
			}
		}
	}

	// Check if any winner claim was made by this worker (all stale).
	// Losing race claims should not classify a worker as stale.
	hasWinnerClaim := false
	for issueID := range claimedAt {
		if winner, ok := winners[issueID]; ok && baseWorkerIdentity(winner) != workerID {
			continue
		}
		hasWinnerClaim = true
		break
	}
	if hasWinnerClaim {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "stale",
			LastOpTime: lastOp,
		}
	}

	// Idle: no claims, but had recent ops within 2*TTL window
	idleWindowSeconds := int64(2 * defaultTTLMinutes * 60)
	if lastOp > 0 && now-lastOp <= idleWindowSeconds {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "idle",
			LastOpTime: lastOp,
		}
	}

	return WorkerStatus{
		WorkerID:   workerID,
		Status:     "idle",
		LastOpTime: lastOp,
	}
}

func claimWinnersByIssue(workers map[string][]ops.Op) map[string]string {
	type issueState struct {
		claimedAt                  int64
		lastHeartbeat              int64
		ttl                        int
		transitioned               bool
		lastClaimingWorkerActivity int64
	}
	claimsByIssue := make(map[string][]ops.Op)
	opsByIssue := make(map[string][]ops.Op)
	for _, allOps := range workers {
		for _, op := range allOps {
			opsByIssue[op.TargetID] = append(opsByIssue[op.TargetID], op)
			if op.Type == ops.OpClaim {
				claimsByIssue[op.TargetID] = append(claimsByIssue[op.TargetID], op)
			}
		}
	}
	winners := make(map[string]string, len(claimsByIssue))
	for issueID, issueOps := range opsByIssue {
		sort.Slice(issueOps, func(i, j int) bool {
			if issueOps[i].Timestamp != issueOps[j].Timestamp {
				return issueOps[i].Timestamp < issueOps[j].Timestamp
			}
			if issueOps[i].WorkerID != issueOps[j].WorkerID {
				return issueOps[i].WorkerID < issueOps[j].WorkerID
			}
			return issueOps[i].Type < issueOps[j].Type
		})
		stateByWorker := make(map[string]*issueState)
		var activeWorker string
		for _, op := range issueOps {
			staleAt := func(workerID string, now int64) bool {
				s := stateByWorker[workerID]
				if s == nil {
					return true
				}
				ttl := s.ttl
				if ttl <= 0 {
					ttl = 60
				}
				return claim.IsClaimStale(s.claimedAt, s.lastHeartbeat, s.lastClaimingWorkerActivity, ttl, now)
			}
			switch op.Type {
			case ops.OpClaim:
				if staleAt(activeWorker, op.Timestamp) {
					activeWorker = op.WorkerID
					stateByWorker[op.WorkerID] = &issueState{
						claimedAt:                  op.Timestamp,
						lastHeartbeat:              op.Timestamp,
						ttl:                        op.Payload.TTL,
						lastClaimingWorkerActivity: op.Timestamp,
					}
				}
			case ops.OpHeartbeat:
				if s := stateByWorker[op.WorkerID]; s != nil && op.Timestamp > s.lastHeartbeat {
					s.lastHeartbeat = op.Timestamp
				}
			case ops.OpTransition:
				// stateByWorker is keyed by workerID, so a transition op only ever
				// touches its own author's state here — by construction this is
				// claiming-worker activity and cannot extend a different worker's claim.
				if s := stateByWorker[op.WorkerID]; s != nil && op.Timestamp > s.lastClaimingWorkerActivity {
					s.lastClaimingWorkerActivity = op.Timestamp
				}
				if op.Payload.To == ops.StatusDone || op.Payload.To == ops.StatusMerged || op.Payload.To == ops.StatusCancelled {
					if s := stateByWorker[op.WorkerID]; s != nil {
						s.transitioned = true
					}
					activeWorker = ""
				}
			}
		}
		if activeWorker != "" {
			if s := stateByWorker[activeWorker]; s != nil && !s.transitioned {
				winners[issueID] = baseWorkerIdentity(activeWorker)
				continue
			}
		}
		claims := claimsByIssue[issueID]
		if len(claims) == 0 {
			continue
		}
		winner := claim.ResolveClaim(claims)
		winners[issueID] = baseWorkerIdentity(winner.WorkerID)
	}
	return winners
}

// lastOpTimestampFromLog returns the timestamp of the most recent op in the list.
func lastOpTimestampFromLog(allOps []ops.Op) int64 {
	var last int64
	for _, op := range allOps {
		if op.Timestamp > last {
			last = op.Timestamp
		}
	}
	return last
}
