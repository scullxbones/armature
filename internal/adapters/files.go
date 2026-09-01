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

// ===== Log File Operations (from ops/log.go) =====

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
	logPath := a.Path
	if len(buf) == 0 {
		return nil
	}

	metaDir, err := appendMetaDir(logPath)
	if err != nil {
		return err
	}
	metaBase := filepath.Join(metaDir, filepath.Base(logPath))

	lock, err := lockLog(metaBase)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }() //nolint:errcheck // close error in defer not actionable

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: internal state path
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // close error in defer not actionable

	markerPath := metaBase + pendingMarkerSuffix
	retry, err := recoverPendingAppend(f, markerPath, buf)
	if err != nil {
		return err
	}
	if retry {
		return nil
	}

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat log %s: %w", logPath, err)
	}
	firstLine, _, _ := bytes.Cut(buf, []byte{'\n'})

	wasTorn := false
	if info.Size() > 0 {
		var tail [1]byte
		if _, err := f.ReadAt(tail[:], info.Size()-1); err != nil {
			return fmt.Errorf("read log tail %s: %w", logPath, err)
		}
		wasTorn = tail[0] != '\n'
	}

	// Only a torn tail is eligible for content-based dedup. A newline-
	// terminated final record was already committed by an unrelated completed
	// call, so an identical record after it must be preserved. Complete retry
	// detection is handled above using the marker's exact byte range.
	duplicate := false
	if wasTorn {
		duplicate, err = lastRecordMatches(f, info.Size(), firstLine)
		if err != nil {
			return fmt.Errorf("read final log record %s: %w", logPath, err)
		}
	}
	if wasTorn {
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return fmt.Errorf("delimit interrupted log record %s: %w", logPath, err)
		}
		info, err = f.Stat()
		if err != nil {
			return fmt.Errorf("stat delimited log %s: %w", logPath, err)
		}
	}

	if !duplicate {
		marker := pendingAppend{Start: info.Size(), Data: buf}
		if err := writePendingMarker(markerPath, marker); err != nil {
			return err
		}
		if _, err := f.Write(buf); err != nil {
			return fmt.Errorf("write to log %s: %w", logPath, err)
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync log %s: %w", logPath, err)
		}
	}

	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pending marker %s: %w", markerPath, err)
	}
	if err := syncDir(filepath.Dir(markerPath)); err != nil {
		return fmt.Errorf("sync pending marker directory %s: %w", markerPath, err)
	}
	return nil
}

// pendingMarkerSuffix names the sidecar file that records the record
// currently being appended, so a crash between the marker write and log
// durability can be recognized as a retry on the next call. See
// AppendLog.Append for the reasoning.
const pendingMarkerSuffix = ".pending"

// appendMetaSubdir holds lock and pending-marker sidecar files for
// AppendLog.Append, kept out of the log's own directory so directory
// listings of ops files (which may match log names by substring, e.g. a
// slot suffix) never pick up a sidecar file instead of the log itself.
const appendMetaSubdir = ".arm-append-meta"

// OpsGitignore is the canonical .gitignore content `arm bootstrap` writes to
// the ops worktree root. It is the single source of truth for what must
// never be committed there — bootstrap.go writes it verbatim, and tests
// reference it directly instead of depending on an on-disk copy.
const OpsGitignore = `# Materialized state — derived from ops logs, regenerated locally by each worker.
# Never commit. See architecture.md §2 (Directory Structure).
state/

# Lock and pending-marker sidecar files for AppendRawLines. Never commit;
# these are ephemeral, worker-local coordination files, not ops state.
**/` + appendMetaSubdir + `/
`

// appendMetaDir returns (creating if needed) the sidecar directory for logPath.
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
		_ = lock.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("lock log %s: %w", logPath, err)
	}
	return lock, nil
}

func writePendingMarker(path string, marker pendingAppend) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("marshal pending marker: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("write pending marker %s: %w", path, err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }() //nolint:errcheck // best-effort cleanup
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close() //nolint:errcheck // cleanup on error path
		return fmt.Errorf("chmod pending marker %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close() //nolint:errcheck // cleanup on error path
		return fmt.Errorf("write pending marker %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close() //nolint:errcheck // cleanup on error path
		return fmt.Errorf("sync pending marker %s: %w", path, err)
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

func syncDir(path string) error {
	dir, err := os.Open(path) //nolint:gosec // internal state path
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }() //nolint:errcheck // close error in defer not actionable
	return dir.Sync()
}

// recoverPendingAppend completes recovery for the exact append described by a
// surviving marker. It returns true only when buf is that already-complete
// append, making a duplicate retry safe to suppress.
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
		// The marker's own append never finished durably (a torn write left a
		// partial, invalid JSONL record on disk, or nothing at all). The
		// prefix already on disk was just verified byte-for-byte against
		// marker.Data, so the remaining suffix is known exactly — completing
		// the write requires no guessing (unlike patching in an assumed
		// delimiter). Finish it from the marker's own recorded bytes so the
		// originally attempted op is never silently lost, then fall through
		// to append the caller's buf: usually a distinct record, but if buf
		// happens to equal marker.Data (an exact retry of the now-completed
		// write), the equality check below suppresses it as a duplicate.
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
		// Now that the marker's own append is durable, buf may turn out to be
		// an exact retry of it (e.g. only the trailing delimiter was torn) —
		// in which case it must be suppressed as a duplicate just like the
		// already-complete case below. Otherwise buf is a distinct record and
		// must still be appended by the normal path.
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

// lastRecordMatches reports whether the final record, whether newline-terminated
// or not, is line. It scans backwards in bounded chunks so appending an op does
// not load the whole append-only log into memory.
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

// ReadLog reads all lines from a log file as raw JSON lines.
func ReadLog(logPath string) ([][]byte, error) {
	return ReadLogFromOffset(logPath, 0)
}

// ReadLogFromOffset reads lines starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) ([][]byte, error) {
	f, err := os.Open(logPath) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // close error in defer not actionable

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	var lines [][]byte
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lines = append(lines, append([]byte{}, line...))
	}
	return lines, scanner.Err()
}

// LineWithOffset represents a line from a log file and its ending byte offset.
type LineWithOffset struct {
	Line      []byte
	EndOffset int64
}

// ReadLogLinesWithOffsets reads lines starting from a byte offset and returns each line
// with the byte offset where it ends (for checkpoint tracking).
func ReadLogLinesWithOffsets(logPath string, startOffset int64) ([]LineWithOffset, error) {
	f, err := os.Open(logPath) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // close error in defer not actionable

	currentOffset := startOffset
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	var lines []LineWithOffset
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

// ===== Materialize State File Operations (from materialize/state.go) =====

// WriteIssueJSON writes a JSON-marshalable issue to a file.
func WriteIssueJSON(issuesDir string, issueID string, data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	path := filepath.Join(issuesDir, issueID+".json")
	return os.WriteFile(path, jsonData, 0o600)
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

// ===== Checkpoint File Operations (from materialize/checkpoint.go) =====

// WriteCheckpointJSON writes a JSON checkpoint to a file.
func WriteCheckpointJSON(path string, data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	return os.WriteFile(path, jsonData, 0o600)
}

// LoadCheckpointJSON reads and unmarshals a checkpoint file.
func LoadCheckpointJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // Return nil for missing checkpoint
		}
		return fmt.Errorf("read checkpoint: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse checkpoint: %w", err)
	}
	return nil
}

// ===== Manifest File Operations (from sources/manifest.go) =====

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
		tmpFile.Close()    //nolint:errcheck,gosec // cleanup on error path
		os.Remove(tmpPath) //nolint:errcheck,gosec // cleanup on error path
		return fmt.Errorf("writing manifest temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec // cleanup on error path
		return fmt.Errorf("closing manifest temp file: %w", err)
	}

	dest := filepath.Join(path, "manifest.json")
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec // cleanup on error path
		return fmt.Errorf("renaming manifest temp file: %w", err)
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

// ===== Config File Operations (from config/config.go) =====

// WriteConfigFile writes JSON config data to a file.
func WriteConfigFile(path string, data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, jsonData, 0o600)
}

// LoadConfigFile reads and unmarshals a config file.
func LoadConfigFile(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return nil
}

// StatFile checks if a file exists and returns true if it does.
func StatFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ===== Plan File Operations (from decompose/plan.go) =====

// ReadPlanFile reads a plan JSON file from the given path.
func ReadPlanFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal state path
	if err != nil {
		return nil, fmt.Errorf("read plan file %s: %w", path, err)
	}
	return data, nil
}

// ===== Traceability Coverage Operations (from traceability/traceability.go) =====

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

// ===== Glob Expansion (for scope validation) =====

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

// ===== Directory Operations (for state directories and ops directories) =====

// MkdirAll creates directories recursively.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// ReadDir lists all entries in a directory.
// Returns empty slice if directory does not exist.
func ReadDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []os.DirEntry{}, nil
		}
		return nil, err
	}
	return entries, nil
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

// ===== Generic File Read/Write (for packages that need raw I/O) =====

// ReadFile reads the entire contents of a file.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // G304: internal state path
}

// WriteFile writes data to a file, creating it if it does not exist.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
