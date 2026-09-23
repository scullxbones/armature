package main

import (
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
	Status      string `json:"status"`
	LastOpTime  int64  `json:"last_op_time"`
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
				s := foldWorkerStatusFromClaimOwnerActivity(workerID, allOps, defaultTTL, now, winners)
				statuses = append(statuses, s)
			}

			sort.Slice(statuses, func(i, j int) bool {
				return statuses[i].WorkerID < statuses[j].WorkerID
			})

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if jsonOut || format == "json" || format == "agent" {
				help := []string{"arm show <id> for the issue a worker is claimed on"}
				if len(statuses) == 0 {
					help = []string{"no worker logs found", "arm worker-init registers a worker identity"}
				}
				return writeNamedEnvelope(cmd.OutOrStdout(), "workers", statuses, help)
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

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON envelope (alias of --format json)")
	return cmd
}

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

func claimingWorkerActivityIfAuthorOwnsLease(author, claimOwner string, ts, last int64) int64 {
	if author != claimOwner {
		return last
	}
	if ts > last {
		return ts
	}
	return last
}

type claimOwnerClocks struct {
	owner                      string
	claimedAt                  int64
	lastHeartbeat              int64
	ttl                        int
	lastClaimingWorkerActivity int64
	transitioned               bool
}

func (c *claimOwnerClocks) recordHeartbeat(ts int64) {
	if ts > c.lastHeartbeat {
		c.lastHeartbeat = ts
	}
}

func (c *claimOwnerClocks) recordTransitionByAuthor(author string, ts int64, to string) {
	c.lastClaimingWorkerActivity = claimingWorkerActivityIfAuthorOwnsLease(author, c.owner, ts, c.lastClaimingWorkerActivity)
	if ops.IsTerminalStatus(to) {
		c.transitioned = true
	}
}

func (c *claimOwnerClocks) recordTransitionAt(ts int64, to string) {
	if ts > c.lastClaimingWorkerActivity {
		c.lastClaimingWorkerActivity = ts
	}
	if ops.IsTerminalStatus(to) {
		c.transitioned = true
	}
}

func (c claimOwnerClocks) lastActivity() claim.LastActivity {
	return claim.FoldLastActivity(c.claimedAt, c.lastHeartbeat, c.lastClaimingWorkerActivity)
}

func foldWorkerStatusFromClaimOwnerActivity(workerID string, allOps []ops.Op, defaultTTLMinutes int, now int64, winners map[string]string) WorkerStatus {
	lastOp := lastOpTimestampFromLog(allOps)

	clocksByIssue := make(map[string]*claimOwnerClocks)

	for _, op := range allOps {
		switch op.Type {
		case ops.OpClaim:
			c := clocksByIssue[op.TargetID]
			if c == nil {
				c = &claimOwnerClocks{}
				clocksByIssue[op.TargetID] = c
			}
			c.owner = op.WorkerID
			c.claimedAt = op.Timestamp
			c.ttl = op.Payload.TTL
			c.lastClaimingWorkerActivity = op.Timestamp
		case ops.OpHeartbeat:
			c := clocksByIssue[op.TargetID]
			if c == nil {
				c = &claimOwnerClocks{}
				clocksByIssue[op.TargetID] = c
			}
			c.recordHeartbeat(op.Timestamp)
		case ops.OpTransition:
			c := clocksByIssue[op.TargetID]
			if c == nil {
				c = &claimOwnerClocks{}
				clocksByIssue[op.TargetID] = c
			}
			c.recordTransitionByAuthor(op.WorkerID, op.Timestamp, op.Payload.To)
		}
	}

	hasWinnerClaim := false
	for issueID, c := range clocksByIssue {
		if c.claimedAt == 0 {
			continue
		}
		if winner, ok := winners[issueID]; ok && baseWorkerIdentity(winner) != workerID {
			continue
		}
		hasWinnerClaim = true
		if c.transitioned {
			continue
		}
		ttl := c.ttl
		if ttl <= 0 {
			ttl = defaultTTLMinutes
		}
		if !claim.IsClaimStale(c.lastActivity(), ttl, now) {
			return WorkerStatus{
				WorkerID:    workerID,
				Status:      "active",
				LastOpTime:  lastOp,
				ActiveIssue: issueID,
			}
		}
	}
	if hasWinnerClaim {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "stale",
			LastOpTime: lastOp,
		}
	}

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
		Status:     "inactive",
		LastOpTime: lastOp,
	}
}

func claimWinnersByIssue(workers map[string][]ops.Op) map[string]string {
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
		stateByWorker := make(map[string]*claimOwnerClocks)
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
				return claim.IsClaimStale(s.lastActivity(), ttl, now)
			}
			switch op.Type {
			case ops.OpClaim:
				if staleAt(activeWorker, op.Timestamp) {
					activeWorker = op.WorkerID
					stateByWorker[op.WorkerID] = &claimOwnerClocks{
						owner:                      op.WorkerID,
						claimedAt:                  op.Timestamp,
						lastHeartbeat:              op.Timestamp,
						ttl:                        op.Payload.TTL,
						lastClaimingWorkerActivity: op.Timestamp,
					}
				}
			case ops.OpHeartbeat:
				if s := stateByWorker[op.WorkerID]; s != nil {
					s.recordHeartbeat(op.Timestamp)
				}
			case ops.OpTransition:
				if s := stateByWorker[op.WorkerID]; s != nil {
					s.recordTransitionByAuthor(op.WorkerID, op.Timestamp, op.Payload.To)
				}
				if ops.IsTerminalStatus(op.Payload.To) {
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

func lastOpTimestampFromLog(allOps []ops.Op) int64 {
	var last int64
	for _, op := range allOps {
		if op.Timestamp > last {
			last = op.Timestamp
		}
	}
	return last
}
