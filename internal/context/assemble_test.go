package context

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
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
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("Assemble returned nil context")
	}

	var contextFilesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "context_files" {
			contextFilesLayer = &ctx.Layers[i]
			break
		}
	}

	if contextFilesLayer == nil {
		t.Fatal("context_files layer not found in assembled context")
	}
	if len(contextFilesLayer.Content) == 0 {
		t.Fatal("context_files layer has empty content")
	}
	if !containsStr(contextFilesLayer.Content, "path/to/file1.txt") {
		t.Errorf("context_files layer missing path/to/file1.txt")
	}
	if !containsStr(contextFilesLayer.Content, "content of file 1") {
		t.Errorf("context_files layer missing content from file1")
	}
	if !containsStr(contextFilesLayer.Content, "path/to/file2.md") {
		t.Errorf("context_files layer missing path/to/file2.md")
	}
	if !containsStr(contextFilesLayer.Content, "Markdown Content") {
		t.Errorf("context_files layer missing content from file2")
	}
	if !containsStr(contextFilesLayer.Content, "missing/file.txt") {
		t.Errorf("context_files layer missing reference to missing file")
	}
}

func containsStr(s, substring string) bool {
	for i := 0; i <= len(s)-len(substring); i++ {
		if s[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
