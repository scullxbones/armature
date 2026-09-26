package adapters

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendLogAppend_REQ_ARCHIMP_S19_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	if err := NewAppendLog(logPath).Append([]byte("{\"a\":1}\n{\"b\":2}\n")); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadLogFromOffset(logPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestAppendLogTornWriteRecovery_REQ_ARCHIMP_S19_T2(t *testing.T) {
	t.Parallel()
	line := []byte(`{"op":"scope-rename"}`)
	retry := append(append([]byte{}, line...), '\n')

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	require.NoError(t, os.WriteFile(logPath, line, 0o600))
	require.NoError(t, NewAppendLog(logPath).Append(retry))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line}, lines)
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, append(line, '\n'), contents)
}

func TestAppendLog_PreservesLegitimateRepeatedCompleteRecord(t *testing.T) {
	t.Parallel()
	line := []byte(`{"op":"scope-rename"}`)
	initial := append(append([]byte{}, line...), '\n')
	repeat := append(append([]byte{}, line...), '\n')

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	require.NoError(t, os.WriteFile(logPath, initial, 0o600))
	require.NoError(t, NewAppendLog(logPath).Append(repeat))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line, line}, lines)
}

func TestAppendLog_PendingMarkerBeforeWriteDoesNotDropRepeatedRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line := []byte(`{"op":"scope-rename"}`)
	initial := append(append([]byte{}, line...), '\n')
	repeat := append(append([]byte{}, line...), '\n')
	require.NoError(t, os.WriteFile(logPath, initial, 0o600))

	require.NoError(t, SimulatePendingMarker(logPath, int64(len(initial)), repeat))
	require.NoError(t, NewAppendLog(logPath).Append(repeat))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line, line}, lines)
}

func TestAppendLog_PendingMarkerWithOnlyDelimiterMissing_DoesNotDuplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line := []byte(`{"op":"scope-rename"}`)
	buf := append(append([]byte{}, line...), '\n')

	require.NoError(t, os.WriteFile(logPath, buf[:len(buf)-1], 0o600))
	require.NoError(t, SimulatePendingMarker(logPath, 0, buf))

	require.NoError(t, NewAppendLog(logPath).Append(buf))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line}, lines)
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, buf, contents)
}

func TestAppendLog_MultiOpPendingMarkerWithOnlyDelimiterMissing_DoesNotDuplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line1 := []byte(`{"op":"scope-rename","id":1}`)
	line2 := []byte(`{"op":"scope-rename","id":2}`)
	buf := append(append(append(append([]byte{}, line1...), '\n'), line2...), '\n')

	require.NoError(t, os.WriteFile(logPath, buf[:len(buf)-1], 0o600))
	require.NoError(t, SimulatePendingMarker(logPath, 0, buf))

	require.NoError(t, NewAppendLog(logPath).Append(buf))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line1, line2}, lines)
	contents, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, buf, contents)
}

func TestAppendLog_TornWriteWithDifferentRetryCompletesPendingRecord_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	priorLine := []byte(`{"op":"prior"}`)
	prior := append(append([]byte{}, priorLine...), '\n')
	require.NoError(t, os.WriteFile(logPath, prior, 0o600))

	intended := []byte(`{"op":"torn-record","field":"value"}` + "\n")
	partial := intended[:10]
	require.NoError(t, os.WriteFile(logPath, append(append([]byte{}, prior...), partial...), 0o600))

	metaDir, err := appendMetaDir(logPath)
	require.NoError(t, err)
	require.NoError(t, writePendingMarker(filepath.Join(metaDir, filepath.Base(logPath))+pendingMarkerSuffix, pendingAppend{
		Start: int64(len(prior)), Data: intended,
	}))

	newRecord := []byte(`{"op":"new-record"}` + "\n")
	require.NoError(t, NewAppendLog(logPath).Append(newRecord))

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)

	require.Equal(t, string(prior)+string(intended)+string(newRecord), string(raw))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{
		priorLine,
		[]byte(`{"op":"torn-record","field":"value"}`),
		[]byte(`{"op":"new-record"}`),
	}, lines)
}

func TestAppendLog_ConcurrentIdenticalAppendsPreserveBoth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line := []byte("{\"op\":\"note\"}\n")

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			errs <- NewAppendLog(logPath).Append(line)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line[:len(line)-1], line[:len(line)-1]}, lines)
}

func TestAppend_PostCommitCloseErrorDoesNotDuplicateOnRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	buf := []byte("{\"op\":\"note\"}\n")
	log := NewAppendLog(logPath)
	log.closeErr = errors.New("close failed")

	firstErr := log.Append(buf)
	if firstErr != nil {
		require.Error(t, log.Append(buf))
	}

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{buf[:len(buf)-1]}, lines)
	require.NoError(t, firstErr)
}

func TestAppendIf_ProceedFalseSkipsWrite_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	wrote, err := NewAppendLog(logPath).AppendIf([]byte("{\"op\":\"transition\"}\n"), func() (bool, error) {
		return false, nil
	})
	require.NoError(t, err)
	assert.False(t, wrote)
	_, err = os.Stat(logPath)
	if err == nil {
		lines, readErr := ReadLogFromOffset(logPath, 0)
		require.NoError(t, readErr)
		assert.Empty(t, lines)
	}
}

func TestAppendIf_ProceedSeesPriorWriter_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line := []byte("{\"op\":\"transition\"}\n")

	start := make(chan struct{})
	wroteCh := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	proceed := func() (bool, error) {
		lines, err := ReadLogFromOffset(logPath, 0)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}
		return len(lines) == 0, nil
	}
	for range 2 {
		wg.Go(func() {
			<-start
			wrote, err := NewAppendLog(logPath).AppendIf(line, proceed)
			errs <- err
			wroteCh <- wrote
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	close(wroteCh)
	for err := range errs {
		require.NoError(t, err)
	}
	wroteTrue := 0
	for wrote := range wroteCh {
		if wrote {
			wroteTrue++
		}
	}
	assert.Equal(t, 1, wroteTrue)
	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line[:len(line)-1]}, lines)
}

func TestAppendLog_DeduplicatesOnlyFinalRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	line := []byte(`{"op":"scope-rename"}`)
	other := []byte(`{"op":"transition"}`)

	initial := append(append(append([]byte{}, line...), '\n'), other...)
	initial = append(initial, '\n')
	require.NoError(t, os.WriteFile(logPath, initial, 0o600))
	require.NoError(t, NewAppendLog(logPath).Append(append(append([]byte{}, line...), '\n')))

	lines, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{line, other, line}, lines)
}

func TestReadLogFromOffset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	if err := os.WriteFile(logPath, []byte("{\"a\":1}\n{\"b\":2}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := ReadLogFromOffset(logPath, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line from offset, got %d", len(lines))
	}
}

func TestReadLog_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := ReadLogFromOffset("/nonexistent/path/x.log", 0)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWorkerIDFromFilename(t *testing.T) {
	t.Parallel()
	cases := []struct{ input, want string }{
		{"3357fe85.log", "3357fe85"},
		{"3357fe85~a.log", "3357fe85"},
		{"/path/to/abc123~t2.log", "abc123"},
	}
	for _, c := range cases {
		if got := WorkerIDFromFilename(c.input); got != c.want {
			t.Errorf("WorkerIDFromFilename(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestWriteAndLoadIssueJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	type payload struct{ X int }
	if err := WriteIssueJSON(dir, "T1", payload{X: 42}); err != nil {
		t.Fatal(err)
	}
	var got payload
	if err := LoadIssueJSON(filepath.Join(dir, "T1.json"), &got); err != nil {
		t.Fatal(err)
	}
	if got.X != 42 {
		t.Fatalf("expected X=42, got %d", got.X)
	}
}

func TestLoadIssueJSON_Missing(t *testing.T) {
	t.Parallel()
	var v struct{}
	err := LoadIssueJSON("/nonexistent/file.json", &v)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadIssuesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "B.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	ids, err := ReadIssuesDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d: %v", len(ids), ids)
	}
}

func TestReadIssuesDir_Missing(t *testing.T) {
	t.Parallel()
	ids, err := ReadIssuesDir("/nonexistent/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty slice, got %v", ids)
	}
}

func TestWriteAndLoadCheckpointJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ckpt.json")
	type ckpt struct{ N int }
	if err := WriteCheckpointJSON(path, ckpt{N: 7}); err != nil {
		t.Fatal(err)
	}
	var got ckpt
	if err := LoadCheckpointJSON(path, &got); err != nil {
		t.Fatal(err)
	}
	if got.N != 7 {
		t.Fatalf("expected N=7, got %d", got.N)
	}
}

func TestLoadCheckpointJSON_Missing(t *testing.T) {
	t.Parallel()
	var v struct{}
	if err := LoadCheckpointJSON("/nonexistent/ckpt.json", &v); err != nil {
		t.Fatal("expected nil for missing checkpoint, got", err)
	}
}

func TestReadWriteManifestFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := []byte(`{"sources":[]}`)
	if err := WriteManifestFile(dir, data); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifestFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("manifest mismatch: %s", got)
	}
}

func TestReadManifestFile_Missing(t *testing.T) {
	t.Parallel()
	got, err := ReadManifestFile("/nonexistent/dir")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for missing manifest")
	}
}

func TestWriteAndReadCacheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteCacheFile(dir, "abc", []byte("cached")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCacheFile(dir, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cached" {
		t.Fatalf("expected 'cached', got %q", got)
	}
}

func TestReadCacheFile_Missing(t *testing.T) {
	t.Parallel()
	got, err := ReadCacheFile(t.TempDir(), "nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for missing cache")
	}
}

func TestWriteAndLoadConfigFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	type cfg struct{ Mode string }
	if err := WriteConfigFile(path, cfg{Mode: "dual"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got cfg
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "dual" {
		t.Fatalf("expected Mode=dual, got %q", got.Mode)
	}
}

func TestStatFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if StatFile(p) {
		t.Fatal("expected false for missing file")
	}
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !StatFile(p) {
		t.Fatal("expected true for existing file")
	}
}

func TestReadPlanFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(p, []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := ReadPlanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"tasks":[]}` {
		t.Fatalf("unexpected plan data: %s", data)
	}
}

func TestReadPlanFile_Missing(t *testing.T) {
	t.Parallel()
	_, err := ReadPlanFile("/nonexistent/plan.json")
	if err == nil {
		t.Fatal("expected error for missing plan file")
	}
}

func TestWriteAndReadCoverageFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "coverage.json")
	type cov struct{ Total int }
	if err := WriteCoverageFile(p, cov{Total: 99}); err != nil {
		t.Fatal(err)
	}
	data, err := ReadCoverageFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty coverage data")
	}
}

func TestReadCoverageFile_Missing(t *testing.T) {
	t.Parallel()
	data, err := ReadCoverageFile("/nonexistent/coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Fatal("expected nil for missing coverage file")
	}
}

func TestReadLogLinesWithOffsets_UnterminatedLastLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	content := []byte("{\"a\":1}\n{\"b\":2}")
	if err := os.WriteFile(logPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := ReadLogLinesWithOffsets(logPath, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	actualFileSize := info.Size()

	lastLineEndOffset := lines[1].EndOffset
	if lastLineEndOffset != actualFileSize {
		t.Errorf("expected last line EndOffset to be %d (file size), got %d", actualFileSize, lastLineEndOffset)
	}

	if string(lines[0].Line) != "{\"a\":1}" {
		t.Errorf("expected first line to be {\"a\":1}, got %s", lines[0].Line)
	}
	if string(lines[1].Line) != "{\"b\":2}" {
		t.Errorf("expected second line to be {\"b\":2}, got %s", lines[1].Line)
	}
}

func TestReadLogLinesWithOffsets_StartOffsetPositive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	content := []byte("{\"a\":1}\n{\"b\":2}\n{\"c\":3}")
	if err := os.WriteFile(logPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := ReadLogLinesWithOffsets(logPath, 8)
	if err != nil {
		t.Fatal(err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	if string(lines[0].Line) != "{\"b\":2}" {
		t.Errorf("expected lines[0].Line to be {\"b\":2}, got %s", lines[0].Line)
	}
	if lines[0].EndOffset != 16 {
		t.Errorf("expected lines[0].EndOffset to be 16, got %d", lines[0].EndOffset)
	}

	if string(lines[1].Line) != "{\"c\":3}" {
		t.Errorf("expected lines[1].Line to be {\"c\":3}, got %s", lines[1].Line)
	}
	if lines[1].EndOffset != 23 {
		t.Errorf("expected lines[1].EndOffset to be 23, got %d", lines[1].EndOffset)
	}
}

func TestListLogFiles_ReturnsLogFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "worker1.log"), []byte("line"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := ListLogFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
}

func TestListLogFiles_MissingDir_ReturnsNil(t *testing.T) {
	t.Parallel()

	files, err := ListLogFiles("/nonexistent/dir/path")
	if err != nil {
		t.Fatal(err)
	}
	if files != nil {
		t.Fatalf("expected nil for missing dir, got %v", files)
	}
}

func TestExpandGlobs_MatchesFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	globs := map[string][]string{
		"issue-01": {filepath.Join(dir, "*.go")},
	}
	result := ExpandGlobs(globs)
	if len(result["issue-01"]) != 1 {
		t.Fatalf("expected 1 match, got %v", result["issue-01"])
	}
}

func TestMkdirAll_CreatesDirectory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "a", "b", "c")

	if err := MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatal("expected directory to be created")
	}
}

func TestStat_ReturnsFileInfo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	info, err := Stat(path)
	if err != nil || info == nil {
		t.Fatalf("expected file info, got err=%v info=%v", err, info)
	}
}

func TestStat_MissingFile_ReturnsNil(t *testing.T) {
	t.Parallel()

	info, err := Stat("/nonexistent/file.go")
	if err != nil || info != nil {
		t.Fatalf("expected nil for missing path, got err=%v info=%v", err, info)
	}
}

func TestReadFile_ReadsContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}

	data, err := ReadFile(path)
	if err != nil || string(data) != "content" {
		t.Fatalf("expected 'content', got err=%v data=%q", err, data)
	}
}

func TestWriteFile_WritesContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := WriteFile(path, []byte("written"), 0600); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil || string(data) != "written" {
		t.Fatalf("expected 'written', got err=%v data=%q", err, data)
	}
}

func TestRemoveIssueJSON_DeletesFileAndToleratesMissing_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "task-01.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"id":"task-01"}`), 0600))

	require.NoError(t, RemoveIssueJSON(dir, "task-01"))
	assert.NoFileExists(t, path)

	assert.NoError(t, RemoveIssueJSON(dir, "task-01"))
}

func TestRemoveIssueJSON_SurfacesUnexpectedError_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(locked, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "task-01.json"), []byte(`{}`), 0600))
	require.NoError(t, os.Chmod(locked, 0555))
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0755); err != nil {
			t.Fatal(err)
		}
	})

	assert.Error(t, RemoveIssueJSON(locked, "task-01"))
}
