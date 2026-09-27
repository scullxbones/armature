package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
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
				defaultTTL = config.DefaultTTLMinutes
			}
			now := time.Now().Unix()

			workers, err := enumerateWorkers(opsDir)
			if err != nil {
				return fmt.Errorf("enumerate workers: %w", err)
			}

			statuses := make([]WorkerStatus, 0, len(workers))
			var allOps []ops.Op
			for _, workerOps := range workers {
				allOps = append(allOps, workerOps...)
			}
			for workerID, workerOps := range workers {
				s := foldWorkerStatusFromClaimOwnerActivity(workerID, workerOps, allOps, defaultTTL, now)
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

func foldWorkerStatusFromClaimOwnerActivity(workerID string, workerOps, allOps []ops.Op, defaultTTL config.TTLMinutes, now int64) WorkerStatus {
	lastOp := lastOpTimestampFromLog(workerOps)
	base := baseWorkerIdentity(workerID)
	seen := map[string]struct{}{}
	var issueIDs []string
	for _, op := range allOps {
		if op.TargetID == "" {
			continue
		}
		if _, ok := seen[op.TargetID]; ok {
			continue
		}
		seen[op.TargetID] = struct{}{}
		issueIDs = append(issueIDs, op.TargetID)
	}
	sort.Strings(issueIDs)

	hasStale := false
	for _, issueID := range issueIDs {
		lease := claim.Owner(allOps, issueID)
		if baseWorkerIdentity(lease.Holder) != base {
			continue
		}
		if lease.Status != ops.StatusClaimed && lease.Status != ops.StatusInProgress {
			continue
		}
		if claim.LeaseLive(lease, now) {
			return WorkerStatus{
				WorkerID:    workerID,
				Status:      "active",
				LastOpTime:  lastOp,
				ActiveIssue: issueID,
			}
		}
		hasStale = true
	}
	if hasStale {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "stale",
			LastOpTime: lastOp,
		}
	}

	idleWindowSeconds := 2 * defaultTTL.Seconds()
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

func lastOpTimestampFromLog(allOps []ops.Op) int64 {
	var last int64
	for _, op := range allOps {
		if op.Timestamp > last {
			last = op.Timestamp
		}
	}
	return last
}
