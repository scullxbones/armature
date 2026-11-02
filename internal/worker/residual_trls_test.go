package worker

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// residualTrlsWord matches the pre-rename CLI token as a whole word so
// substrings like "controls" do not false-positive.
var residualTrlsWord = regexp.MustCompile(`(?i)\btrls\b`)

// TestNoResidualTrlsInInternalGo_REQ_BUG_1783480206 is the class lock for the
// ARM rename leftover: every *.go under internal/ must be free of the old
// CLI name. Scoped product sites (TUI brand, worker-init hint) and fixtures
// are the same failure mode — one grep class, not per-file spot fixes.
func TestNoResidualTrlsInInternalGo_REQ_BUG_1783480206(t *testing.T) {
	t.Parallel()

	root := findModuleRoot(t)
	internalDir := filepath.Join(root, "internal")
	var hits []string
	err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			// Production sources only: tests may name the banned token in
			// assertions without counting as a residual product string.
			return nil
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path from WalkDir under internal/
		if readErr != nil {
			return readErr
		}
		if !residualTrlsWord.Match(data) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for i, line := range strings.Split(string(data), "\n") {
			if residualTrlsWord.MatchString(line) {
				hits = append(hits, filepath.ToSlash(rel)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, hits, "residual pre-rename CLI name trls in internal Go:\n%s", strings.Join(hits, "\n"))
}

func TestGetWorkerID_ErrorNamesArmWorkerInit_REQ_BUG_1783480206(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	_, err := GetWorkerID(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arm worker-init")
	assert.NotContains(t, err.Error(), "trls")
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(thisFile)
	for range 12 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find module root (go.mod)")
	return ""
}
