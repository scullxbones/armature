package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSFileReader_JoinsRoot_REQ_ARCHIMP_S16_T1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))

	testContent := []byte("test content")
	testFile := filepath.Join(subdir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, testContent, 0644))

	reader := &OSFileReader{Root: subdir}

	content, err := reader.ReadFile("test.txt")
	require.NoError(t, err)
	require.Equal(t, testContent, content)
}

// fakeReader is an in-memory FileReader implementation for testing.
type fakeReader struct {
	files map[string][]byte
}

func (f *fakeReader) ReadFile(relPath string) ([]byte, error) {
	if content, ok := f.files[relPath]; ok {
		return content, nil
	}
	return nil, fmt.Errorf("file not found: %s", relPath)
}

func TestAssemble_ContextFilesLayerUsesReader_REQ_ARCHIMP_S16_T2(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{
		files: map[string][]byte{
			"path/to/file1.txt": []byte("content of file 1"),
			"path/to/file2.md":  []byte("# Markdown Content\n\nThis is a test."),
		},
	}

	state := &materialize.State{
		Issues: map[string]*materialize.Issue{
			"TEST-1": {
				ID:    "TEST-1",
				Title: "Test Issue",
				Type:  "task",
				ContextFiles: []string{
					"path/to/file1.txt",
					"path/to/file2.md",
					"missing/file.txt",
				},
				BlockedBy: []string{},
				Blocks:    []string{},
				Children:  []string{},
			},
		},
	}

	ctx, err := Assemble("TEST-1", state, reader)
	require.NoError(t, err, "Assemble failed")
	require.NotNil(t, ctx, "Assemble returned nil context")

	var contextFilesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "context_files" {
			contextFilesLayer = &ctx.Layers[i]
			break
		}
	}

	require.NotNil(t, contextFilesLayer, "context_files layer not found in assembled context")
	require.NotEmpty(t, contextFilesLayer.Content, "context_files layer has empty content")
	assert.True(t, strings.Contains(contextFilesLayer.Content, "path/to/file1.txt"), "context_files layer missing path/to/file1.txt")
	assert.True(t, strings.Contains(contextFilesLayer.Content, "content of file 1"), "context_files layer missing content from file1")
	assert.True(t, strings.Contains(contextFilesLayer.Content, "path/to/file2.md"), "context_files layer missing path/to/file2.md")
	assert.True(t, strings.Contains(contextFilesLayer.Content, "Markdown Content"), "context_files layer missing content from file2")
	assert.True(t, strings.Contains(contextFilesLayer.Content, "missing/file.txt"), "context_files layer missing reference to missing file")
}
