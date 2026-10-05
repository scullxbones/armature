package materialize

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func TestReplayFixtureCorpus_REQ_TOPTIER_S6_T2(t *testing.T) {
	t.Parallel()

	dir := fixtureDir(t)
	opsPath := filepath.Join(dir, "ops.jsonl")
	goldenPath := filepath.Join(dir, "golden-state.json")

	allOps := readFixtureOps(t, opsPath)
	require.NotEmpty(t, allOps, "v1 corpus must contain ops")
	for i, op := range allOps {
		require.Equal(t, ops.CurrentSchemaVersion, op.SchemaVersion, "corpus op %d must be schema v1", i)
	}

	state := NewState()
	require.NoError(t, ApplyOpsSorted(state, allOps))

	got, err := json.MarshalIndent(state.Issues, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	if os.Getenv("UPDATE_OPS_FIXTURE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		t.Logf("updated golden at %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "committed golden-state.json is required")
	if !bytes.Equal(got, want) {
		t.Fatalf("v1 fixture replay diverged from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "testdata", "v1")
}

func readFixtureOps(t *testing.T, path string) []ops.Op {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	var out []ops.Op
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		op, err := ops.ParseLine(line)
		require.NoErrorf(t, err, "ops.jsonl line %d", lineNo)
		out = append(out, op)
	}
	require.NoError(t, scanner.Err())
	return out
}
