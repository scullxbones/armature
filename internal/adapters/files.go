// Package adapters provides boundary adapters for external concerns like file I/O.
// All file read/write operations from core packages are consolidated here.
package adapters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/filelock"
)

func closeOrKeep(errp *error, c io.Closer) {
	if cerr := c.Close(); cerr != nil && *errp == nil {
		*errp = cerr
	}
}

func closeOrKeepErr(primary error, c io.Closer) error {
	if cerr := c.Close(); cerr != nil {
		return errors.Join(primary, cerr)
	}
	return primary
}

func removeOrKeepErr(primary error, path string) error {
	if rerr := os.Remove(path); rerr != nil && !os.IsNotExist(rerr) {
		return errors.Join(primary, rerr)
	}
	return primary
}

// ListLogFiles finds all *.log files in the opsDir directory.
// Returns their absolute paths.
func ListLogFiles(opsDir string) ([]string, error) {
	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			logFiles = append(logFiles, filepath.Join(opsDir, entry.Name()))
		}
	}
	return logFiles, nil
}

// AppendLog appends raw, pre-formatted JSONL lines to a single log file,
// guarding against crash-induced corruption with a .pending marker
// protocol (see Append). Construct one fresh per call site with
// NewAppendLog; it holds no state beyond the target path.
type AppendLog struct {
	Path string
}

// NewAppendLog constructs an AppendLog for the given log file path.
func NewAppendLog(path string) *AppendLog {
	return &AppendLog{Path: path}
}

// Append appends raw bytes to the log file (for pre-formatted JSONL lines).
//
// A process can die mid-append: after writing part (or all) of an operation
// but before writing its JSONL delimiter, or even after the delimiter but
// before the caller learns the append succeeded and retries. Byte content
// alone cannot safely tell such a retry apart from a legitimate second append
// that happens to serialize to the same bytes (e.g. two identical notes from
// one worker within the same nowEpoch() second) — both leave an identical
// final record. So retry intent is tracked explicitly with a marker file
// written before the record is durable and removed once it is. The marker
// records both the append offset and the complete buffer, so recovery can
// identify the exact attempted byte range rather than mistaking an earlier,
// identical record for a retry. Calls for one log are serialized with an
// advisory lock so they cannot overwrite or remove each other's marker.
func (a *AppendLog) Append(buf []byte) error {
	_, err := a.AppendIf(buf, nil)
	return err
}

// AppendIf appends buf unless proceed returns false. proceed runs while the
// per-log lock is held, after pending-marker recovery and before the write,
// so a caller can revalidate an idempotency decision against the durable log
// without a TOCTOU gap versus this file's next append. A nil proceed always
// writes. wrote is false when the call skipped (empty buf, recovered exact
// retry, or proceed returned false).
func (a *AppendLog) AppendIf(buf []byte, proceed func() (bool, error)) (wrote bool, err error) {
	logPath := a.Path
	if len(buf) == 0 {
		return false, nil
	}

	metaDir, err := appendMetaDir(logPath)
	if err != nil {
		return false, err
	}
	metaBase := filepath.Join(metaDir, filepath.Base(logPath))

	lock, err := lockLog(metaBase)
	if err != nil {
		return false, err
	}
	defer func() { closeOrKeep(&err, lock) }()

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: internal state path
	if err != nil {
		return false, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { closeOrKeep(&err, f) }()

	markerPath := metaBase + pendingMarkerSuffix
	retry, err := recoverPendingAppend(f, markerPath, buf)
	if err != nil {
		return false, err
	}
	if retry {
		return false, nil
	}

	if proceed != nil {
		ok, err := proceed()
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat log %s: %w", logPath, err)
	}
	firstLine, _, _ := bytes.Cut(buf, []byte{'\n'})

	wasTorn := false
	if info.Size() > 0 {
		var tail [1]byte
		if _, err := f.ReadAt(tail[:], info.Size()-1); err != nil {
			return false, fmt.Errorf("read log tail %s: %w", logPath, err)
		}
		wasTorn = tail[0] != '\n'
	}

	duplicate := false
	if wasTorn {
		duplicate, err = lastRecordMatches(f, info.Size(), firstLine)
		if err != nil {
			return false, fmt.Errorf("read final log record %s: %w", logPath, err)
		}
	}
	if wasTorn {
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return false, fmt.Errorf("delimit interrupted log record %s: %w", logPath, err)
		}
		info, err = f.Stat()
		if err != nil {
			return false, fmt.Errorf("stat delimited log %s: %w", logPath, err)
		}
	}

	if !duplicate {
		marker := pendingAppend{Start: info.Size(), Data: buf}
		if err := writePendingMarker(markerPath, marker); err != nil {
			return false, err
		}
		if _, err := f.Write(buf); err != nil {
			return false, fmt.Errorf("write to log %s: %w", logPath, err)
		}
		if err := f.Sync(); err != nil {
			return false, fmt.Errorf("sync log %s: %w", logPath, err)
		}
	}

	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove pending marker %s: %w", markerPath, err)
	}
	if err := syncDir(filepath.Dir(markerPath)); err != nil {
		return false, fmt.Errorf("sync pending marker directory %s: %w", markerPath, err)
	}
	return true, nil
}

const pendingMarkerSuffix = ".pending"

const appendMetaSubdir = ".arm-append-meta"

// OpsGitignore is the ignore body `arm bootstrap` writes into the ops worktree
// .gitignore (via ops.GenerateOpsGitignore, which prefixes scaffolding-version).
// It is the single source of truth for what must never be committed there —
// bootstrap.go writes it through the generator, and tests reference this
// constant directly instead of depending on an on-disk copy.
const OpsGitignore = `# Materialized state — derived from ops logs, regenerated locally by each worker.
# Never commit. See architecture.md §2 (Directory Structure).
state/

# Gate-run logs and reviewer assessment JSON. Local sidecars; durable
# records are gate-evidence and assessment-attested ops. Never commit.
gates/
review/

# Hook templates, regenerated from this binary's constants on every bootstrap
# and read only by the bootstrap that wrote them. Local; never commit, so two
# clones on different versions cannot fight over them (architecture.md I3).
hooks/*.sh.template

# Lock and pending-marker sidecar files for AppendRawLines. Never commit;
# these are ephemeral, worker-local coordination files, not ops state.
**/` + appendMetaSubdir + `/
`

func appendMetaDir(logPath string) (string, error) {
	dir := filepath.Join(filepath.Dir(logPath), appendMetaSubdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create append metadata dir %s: %w", dir, err)
	}
	return dir, nil
}

type pendingAppend struct {
	Start int64  `json:"start"`
	Data  []byte `json:"data"`
}

func lockLog(logPath string) (*os.File, error) {
	lock, err := os.OpenFile(logPath+".lock", os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // internal state path
	if err != nil {
		return nil, fmt.Errorf("open log lock %s: %w", logPath, err)
	}
	if err := filelock.Lock(lock); err != nil {
		return nil, closeOrKeepErr(fmt.Errorf("lock log %s: %w", logPath, err), lock)
	}
	return lock, nil
}

func writePendingMarker(path string, marker pendingAppend) (err error) {
	data, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("marshal pending marker: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("write pending marker %s: %w", path, err)
	}
	tmpPath := f.Name()
	defer func() {
		if rerr := os.Remove(tmpPath); rerr != nil && !os.IsNotExist(rerr) && err == nil {
			err = rerr
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return closeOrKeepErr(fmt.Errorf("chmod pending marker %s: %w", path, err), f)
	}
	if _, err := f.Write(data); err != nil {
		return closeOrKeepErr(fmt.Errorf("write pending marker %s: %w", path, err), f)
	}
	if err := f.Sync(); err != nil {
		return closeOrKeepErr(fmt.Errorf("sync pending marker %s: %w", path, err), f)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close pending marker %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace pending marker %s: %w", path, err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync pending marker directory %s: %w", path, err)
	}
	return nil
}

func syncDir(path string) (err error) {
	dir, err := os.Open(path) //nolint:gosec // internal state path
	if err != nil {
		return err
	}
	defer func() { closeOrKeep(&err, dir) }()
	return dir.Sync()
}

func recoverPendingAppend(f *os.File, markerPath string, buf []byte) (bool, error) {
	data, err := os.ReadFile(markerPath) //nolint:gosec // internal state path
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending marker %s: %w", markerPath, err)
	}
	var marker pendingAppend
	if err := json.Unmarshal(data, &marker); err != nil {
		return false, fmt.Errorf("decode pending marker %s: %w", markerPath, err)
	}
	if marker.Start < 0 || len(marker.Data) == 0 {
		return false, fmt.Errorf("invalid pending marker %s", markerPath)
	}
	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat log %s: %w", f.Name(), err)
	}
	if marker.Start > info.Size() {
		return false, fmt.Errorf("pending marker %s starts beyond log end", markerPath)
	}
	available := min(int64(len(marker.Data)), info.Size()-marker.Start)
	if available > 0 {
		actual := make([]byte, available)
		if _, err := f.ReadAt(actual, marker.Start); err != nil {
			return false, fmt.Errorf("read pending log bytes %s: %w", f.Name(), err)
		}
		if !bytes.Equal(actual, marker.Data[:available]) {
			return false, fmt.Errorf("pending marker %s does not match log tail", markerPath)
		}
	}
	complete := available == int64(len(marker.Data))
	if !complete {
		remaining := marker.Data[available:]
		if len(remaining) > 0 {
			if _, err := f.Write(remaining); err != nil {
				return false, fmt.Errorf("complete torn log write %s: %w", f.Name(), err)
			}
			if err := f.Sync(); err != nil {
				return false, fmt.Errorf("sync completed log %s: %w", f.Name(), err)
			}
		}
		if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove pending marker %s: %w", markerPath, err)
		}
		if err := syncDir(filepath.Dir(markerPath)); err != nil {
			return false, fmt.Errorf("sync pending marker directory %s: %w", markerPath, err)
		}
		return bytes.Equal(marker.Data, buf), nil
	}
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove pending marker %s: %w", markerPath, err)
	}
	if err := syncDir(filepath.Dir(markerPath)); err != nil {
		return false, fmt.Errorf("sync pending marker directory %s: %w", markerPath, err)
	}
	return bytes.Equal(marker.Data, buf), nil
}

func lastRecordMatches(f *os.File, size int64, line []byte) (bool, error) {
	if size == 0 {
		return false, nil
	}

	end := size
	var tail [1]byte
	if _, err := f.ReadAt(tail[:], size-1); err != nil {
		return false, err
	}
	if tail[0] == '\n' {
		end--
	}
	if end == 0 {
		return false, nil
	}

	const scanChunkSize = 4 << 10

	tailStart := int64(0)
	for scanEnd := end; scanEnd > 0; {
		start := max(scanEnd-scanChunkSize, 0)
		chunk := make([]byte, scanEnd-start)
		if _, err := f.ReadAt(chunk, start); err != nil {
			return false, err
		}
		if newline := bytes.LastIndexByte(chunk, '\n'); newline >= 0 {
			tailStart = start + int64(newline) + 1
			break
		}
		scanEnd = start
	}

	if end-tailStart != int64(len(line)) {
		return false, nil
	}
	record := make([]byte, len(line))
	if _, err := f.ReadAt(record, tailStart); err != nil {
		return false, err
	}
	return bytes.Equal(record, line), nil
}

// ReadLogFromOffset reads lines starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) (lines [][]byte, err error) {
	f, err := os.Open(logPath) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { closeOrKeep(&err, f) }()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lines = append(lines, append([]byte{}, line...))
	}
	err = scanner.Err()
	return lines, err
}

// LineWithOffset represents a line from a log file and its ending byte offset.
type LineWithOffset struct {
	Line      []byte
	EndOffset int64
}

// ReadLogLinesWithOffsets reads lines starting from a byte offset and returns each line
// with the byte offset where it ends (for checkpoint tracking).
func ReadLogLinesWithOffsets(logPath string, startOffset int64) (lines []LineWithOffset, err error) {
	f, err := os.Open(logPath) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { closeOrKeep(&err, f) }()

	currentOffset := startOffset
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	reader := bufio.NewReaderSize(f, 1<<20)
	for {
		rawLine, err := reader.ReadBytes('\n')
		if len(rawLine) > 0 {
			currentOffset += int64(len(rawLine))
			line := bytes.TrimRight(rawLine, "\r\n")
			if len(line) > 0 {
				lineCopy := append([]byte{}, line...)
				lines = append(lines, LineWithOffset{
					Line:      lineCopy,
					EndOffset: currentOffset,
				})
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return lines, nil
}

// WorkerIDFromFilename extracts the worker ID from a log filename.
// Plain log:   "3357fe85.log"   -> "3357fe85"
// Slotted log: "3357fe85~a.log" -> "3357fe85"  (slot suffix stripped)
func WorkerIDFromFilename(logPath string) string {
	base := filepath.Base(logPath)
	name := strings.TrimSuffix(base, ".log")
	if idx := strings.Index(name, "~"); idx >= 0 {
		name = name[:idx]
	}
	return name
}

func writeJSONFile(path string, data any, kind string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", kind, err)
	}
	return os.WriteFile(path, jsonData, 0o600)
}

// WriteIssueJSON writes a JSON-marshalable issue to a file.
func WriteIssueJSON(issuesDir string, issueID string, data any) error {
	return writeJSONFile(filepath.Join(issuesDir, issueID+".json"), data, "issue")
}

// RemoveIssueJSON deletes an issue's state file. A file that is already gone
// is not an error: the caller's goal is that the snapshot no longer exist.
func RemoveIssueJSON(issuesDir string, issueID string) error {
	path := filepath.Join(issuesDir, issueID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadIssueJSON reads a JSON file and unmarshals it into the provided struct.
func LoadIssueJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ReadIssuesDir lists all .json files in the directory.
// Returns a slice of filenames without the .json extension.
func ReadIssuesDir(issuesDir string) ([]string, error) {
	var issueIDs []string

	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return issueIDs, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		issueID := strings.TrimSuffix(entry.Name(), ".json")
		issueIDs = append(issueIDs, issueID)
	}

	return issueIDs, nil
}

// WriteCheckpointJSON writes a JSON checkpoint to a file.
func WriteCheckpointJSON(path string, data any) error {
	return writeJSONFile(path, data, "checkpoint")
}

// LoadCheckpointJSON reads and unmarshals a checkpoint file.
func LoadCheckpointJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read checkpoint: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse checkpoint: %w", err)
	}
	return nil
}

// ReadManifestFile reads a manifest.json file from the given directory.
// If the file does not exist, it returns nil, nil.
func ReadManifestFile(path string) ([]byte, error) {
	filePath := filepath.Join(path, "manifest.json")
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: internal state path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	return data, nil
}

// WriteManifestFile writes data atomically to manifest.json in the given directory.
func WriteManifestFile(path string, data []byte) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmpFile, err := os.CreateTemp(path, "manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("writing manifest temp file: %w", removeOrKeepErr(closeOrKeepErr(err, tmpFile), tmpPath))
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing manifest temp file: %w", removeOrKeepErr(err, tmpPath))
	}

	dest := filepath.Join(path, "manifest.json")
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("renaming manifest temp file: %w", removeOrKeepErr(err, tmpPath))
	}

	return nil
}

// WriteCacheFile writes raw bytes to a cache file named <id>.cache in path.
func WriteCacheFile(path string, id string, data []byte) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cacheFile := filepath.Join(path, id+".cache")
	if err := os.WriteFile(cacheFile, data, 0o600); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	return nil
}

// ReadCacheFile reads the cache file named <id>.cache from path.
// If the file does not exist, it returns nil, nil.
func ReadCacheFile(path string, id string) ([]byte, error) {
	cacheFile := filepath.Join(path, id+".cache")
	data, err := os.ReadFile(cacheFile) //nolint:gosec // G304: internal state path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cache file: %w", err)
	}
	return data, nil
}

// WriteConfigFile writes JSON config data to a file.
func WriteConfigFile(path string, data any) error {
	return writeJSONFile(path, data, "config")
}

// StatFile checks if a file exists and returns true if it does.
func StatFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadPlanFile reads a plan JSON file from the given path.
func ReadPlanFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("read plan file %s: %w", path, err)
	}
	return data, nil
}

// WriteCoverageFile writes coverage data to a file (atomic write via temp file).
func WriteCoverageFile(path string, data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, jsonData, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadCoverageFile reads coverage data from a file.
// If the file does not exist, it returns nil, nil.
func ReadCoverageFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// ExpandGlobs expands a set of glob patterns and returns matching file paths.
// Returns a map from issue ID to matching file paths.
func ExpandGlobs(globs map[string][]string) map[string][]string {
	result := make(map[string][]string)
	for id, globList := range globs {
		var matches []string
		seen := make(map[string]bool)
		for _, glob := range globList {
			expanded, err := filepath.Glob(glob)
			if err != nil {
				continue
			}
			for _, path := range expanded {
				if !seen[path] {
					matches = append(matches, path)
					seen[path] = true
				}
			}
		}
		result[id] = matches
	}
	return result
}

// MkdirAll creates directories recursively.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Stat returns file info for a path.
// Returns nil if path does not exist.
func Stat(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

// ReadFile reads the entire contents of a file.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // G304: internal state path
}

// WriteFile writes data to a file, creating it if it does not exist.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
