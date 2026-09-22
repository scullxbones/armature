package claim

import "github.com/scullxbones/armature/internal/ops"

// WorktreeRestoreClearingEmptyPrior maps a prior claim worktree path onto ops.WorktreeRestore.
func WorktreeRestoreClearingEmptyPrior(priorPath string) ops.WorktreeRestore {
	if priorPath == "" {
		return ops.WorktreeRestore{Action: ops.WorktreeClear}
	}
	return ops.WorktreeRestore{Action: ops.WorktreeSet, Path: priorPath}
}
