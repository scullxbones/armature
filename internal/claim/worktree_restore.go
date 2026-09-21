package claim

import "github.com/scullxbones/armature/internal/ops"

// WorktreeRestore maps a prior claim worktree path to the compensation
// tri-state. Empty prior is Clear, not Unchanged: omitting both wire flags
// would leave the failed claim's path in place.
func WorktreeRestore(priorPath string) ops.WorktreeRestore {
	if priorPath == "" {
		return ops.WorktreeRestore{Action: ops.WorktreeClear}
	}
	return ops.WorktreeRestore{Action: ops.WorktreeSet, Path: priorPath}
}

// WorktreeLeave is the third tri-state: neither wire flag (normal transitions).
func WorktreeLeave() ops.WorktreeRestore {
	return ops.WorktreeRestore{Action: ops.WorktreeUnchanged}
}
