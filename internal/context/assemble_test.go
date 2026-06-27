package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOSFileReader_JoinsRoot_REQ_ARCHIMP_S16_T1(t *testing.T) {
	t.Parallel()
	// Create a temporary directory structure
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))

	// Write a test file
	testContent := []byte("test content")
	testFile := filepath.Join(subdir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, testContent, 0644))

	// Create an OSFileReader with the root set to the subdirectory
	reader := &OSFileReader{Root: subdir}

	// Read the file using a relative path
	content, err := reader.ReadFile("test.txt")
	require.NoError(t, err)
	require.Equal(t, testContent, content)
}
