package ops

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLiveArmatureOpsCorpusLoads_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	list, err := exec.CommandContext(t.Context(), "git", "-C", root, "ls-tree", "-r", "--name-only", "origin/_armature", "ops").Output()
	if err != nil {
		t.Skipf("origin/_armature not in this clone (%v); this test git-shows origin/_armature:ops/*.log and LoadFromDirValidated against that tree", err)
	}

	dest := t.TempDir()
	opsDir := filepath.Join(dest, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	var logs int
	var lines int
	for _, name := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		if name == "" || !strings.HasSuffix(name, ".log") {
			continue
		}
		raw, showErr := exec.CommandContext(t.Context(), "git", "-C", root, "show", "origin/_armature:"+name).Output()
		require.NoError(t, showErr, "git show %s", name)
		base := filepath.Base(name)
		require.NoError(t, os.WriteFile(filepath.Join(opsDir, base), raw, 0o644))
		logs++
		require.True(t, len(raw) == 0 || raw[len(raw)-1] == '\n', "%s must end with a newline", base)
		for i, line := range splitPhysicalLog(raw) {
			if len(line.Raw) == 0 {
				continue
			}
			lines++
			var arr []json.RawMessage
			require.NoError(t, json.Unmarshal(line.Raw, &arr), "%s:%d must be a JSON array", base, i+1)
			require.GreaterOrEqual(t, len(arr), 5, "%s:%d must have at least 5 fields", base, i+1)
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
