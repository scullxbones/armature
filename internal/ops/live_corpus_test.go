package ops

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLiveArmatureOpsCorpusLoads_REQ_AGTFND(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.CommandContext(t.Context(), "git", "archive", "origin/_armature", "ops")
	cmd.Dir = root
	archive, err := cmd.Output()
	if err != nil {
		t.Skipf("origin/_armature not in this clone (%v); this test git-archives origin/_armature ops/*.log and LoadFromDirValidated against that tree", err)
	}

	dest := t.TempDir()
	require.NoError(t, extractTar(archive, dest))
	opsDir := filepath.Join(dest, "ops")

	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	var logs int
	var lines int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		logs++
		raw, readErr := os.ReadFile(filepath.Join(opsDir, e.Name())) //nolint:gosec // test corpus
		require.NoError(t, readErr)
		require.True(t, len(raw) == 0 || raw[len(raw)-1] == '\n', "%s must end with a newline", e.Name())
		for i, line := range splitPhysicalLog(raw) {
			if len(line.Raw) == 0 {
				continue
			}
			lines++
			var arr []json.RawMessage
			require.NoError(t, json.Unmarshal(line.Raw, &arr), "%s:%d must be a JSON array", e.Name(), i+1)
			require.GreaterOrEqual(t, len(arr), 5, "%s:%d must have at least 5 fields", e.Name(), i+1)
		}
	}
	require.Greater(t, logs, 0, "expected ops/*.log in origin/_armature")
	require.Greater(t, lines, 0)

	_, err = LoadFromDirValidated(opsDir)
	require.NoError(t, err, "live _armature corpus must load under the strict parser")
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func extractTar(archive []byte, dest string) error {
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(dest, hdr.Name), 0o755); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(dest, hdr.Name) //nolint:gosec // test archive from git
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec
		if err != nil {
			return err
		}
		if _, err := io.CopyN(f, tr, hdr.Size); err != nil && err != io.EOF {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
}
