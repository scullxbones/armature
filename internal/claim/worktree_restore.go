package claim

import "github.com/scullxbones/armature/internal/ops"

func worktreeRestoreClearingEmptyPrior(priorPath string) ops.WorktreeRestore {
	if priorPath == "" {
		return ops.WorktreeRestore{Action: ops.WorktreeClear}
	}
	return ops.WorktreeRestore{Action: ops.WorktreeSet, Path: priorPath}
}
