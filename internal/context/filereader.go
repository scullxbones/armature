package context

import (
	"os"
	"path/filepath"
)

type FileReader interface {
	ReadFile(relPath string) ([]byte, error)
}

type OSFileReader struct {
	Root string
}

func (r *OSFileReader) ReadFile(relPath string) ([]byte, error) {
	fullPath := filepath.Join(r.Root, relPath)
	return os.ReadFile(fullPath) //nolint:gosec // G304: path joins repo root with relative path from issue definition
}
