package materialize

// HistoryReader is the read-only git history boundary used for replaying
// materialized state at a commit without depending on a concrete git adapter.
type HistoryReader interface {
	ListFilesAtCommit(sha string) ([]string, error)
	ShowFileAtCommit(sha, path string) ([]byte, error)
}
