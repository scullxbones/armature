package context

import (
	"os"
	"path/filepath"
)

// FileReader defines an interface for reading files by relative path.
type FileReader interface {
	ReadFile(relPath string) ([]byte, error)
}

// OSFileReader reads files from the operating system filesystem.
type OSFileReader struct {
	Root string
}

// ReadFile joins the Root directory with the relative path and reads the file.
func (r *OSFileReader) ReadFile(relPath string) ([]byte, error) {
	fullPath := filepath.Join(r.Root, relPath)
	return os.ReadFile(fullPath) //nolint:gosec // G304: path joins repo root with relative path from issue definition
}
