package ops

// WorktreeAction is the tri-state worktree restore encoded onto Payload.
// Dual wire flags (WorktreePath + ClearWorktreePath) are the JSONL encoding,
// not the writer contract: empty WorktreePath means leave, not clear.
type WorktreeAction uint8

const (
	WorktreeUnchanged WorktreeAction = iota
	WorktreeClear
	WorktreeSet
)

// WorktreeRestore is the typed tri-state (clear | set | leave/unchanged).
type WorktreeRestore struct {
	Action WorktreeAction
	Path   string
}

// EncodeWorktree maps the tri-state onto the historical payload flags.
// Clear takes the clear_worktree_path bit. Set writes a non-empty path.
// Unchanged writes neither. A Set with an empty path encodes as Clear
// because an empty worktree_path is indistinguishable from omitempty leave.
func EncodeWorktree(w WorktreeRestore) (worktreePath string, clear bool) {
	switch w.Action {
	case WorktreeClear:
		return "", true
	case WorktreeSet:
		if w.Path == "" {
			return "", true
		}
		return w.Path, false
	default:
		return "", false
	}
}

// DecodeWorktreeRestore reads the wire flags. ClearWorktreePath wins when both
// are set, matching applyTransition precedence.
func DecodeWorktreeRestore(p Payload) WorktreeRestore {
	if p.ClearWorktreePath {
		return WorktreeRestore{Action: WorktreeClear}
	}
	if p.WorktreePath != "" {
		return WorktreeRestore{Action: WorktreeSet, Path: p.WorktreePath}
	}
	return WorktreeRestore{Action: WorktreeUnchanged}
}

// Apply returns the worktree path after this restore against current.
func (w WorktreeRestore) Apply(current string) string {
	switch w.Action {
	case WorktreeClear:
		return ""
	case WorktreeSet:
		return w.Path
	default:
		return current
	}
}

// Compensation is the typed claim-compensation payload (Arena B graft).
// Encode is the writer contract; callers do not set dual worktree flags.
type Compensation struct {
	To                                string
	RestoreClaim                      bool
	RestoreClaimedBy                  string
	RestoreClaimedAt                  int64
	RestoreClaimTTL                   int
	RestoreLastHeartbeat              int64
	RestoreLastClaimingWorkerActivity int64
	RestoreClaimToken                 string
	IfClaimToken                      string
	Worktree                          WorktreeRestore
}

// Encode writes Compensation onto the append-only Payload schema.
func (c Compensation) Encode() Payload {
	path, clear := EncodeWorktree(c.Worktree)
	return Payload{
		To:                                c.To,
		RestoreClaim:                      c.RestoreClaim,
		RestoreClaimedBy:                  c.RestoreClaimedBy,
		RestoreClaimedAt:                  c.RestoreClaimedAt,
		RestoreClaimTTL:                   c.RestoreClaimTTL,
		RestoreLastHeartbeat:              c.RestoreLastHeartbeat,
		RestoreLastClaimingWorkerActivity: c.RestoreLastClaimingWorkerActivity,
		RestoreClaimToken:                 c.RestoreClaimToken,
		IfClaimToken:                      c.IfClaimToken,
		WorktreePath:                      path,
		ClearWorktreePath:                 clear,
	}
}
