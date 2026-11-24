package materialize

type historyReader interface {
	ListFilesAtCommit(sha string) ([]string, error)
	ShowFileAtCommit(sha, path string) ([]byte, error)
}
