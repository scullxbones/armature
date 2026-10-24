package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type fileReader interface {
	ReadFile(relPath string) ([]byte, error)
}

type osFileReader struct {
	Root string
}

func (r *osFileReader) ReadFile(relPath string) ([]byte, error) {
	fullPath := filepath.Join(r.Root, relPath)
	return os.ReadFile(fullPath) //nolint:gosec // G304: path joins repo root with relative path from issue definition
}

type fsFileReader struct {
	fs fs.FS
}

func (r fsFileReader) ReadFile(relPath string) ([]byte, error) {
	return fs.ReadFile(r.fs, filepath.ToSlash(relPath))
}

type emptyFiles struct{}

func (emptyFiles) ReadFile(relPath string) ([]byte, error) {
	return nil, fmt.Errorf("file not found: %s", relPath)
}
