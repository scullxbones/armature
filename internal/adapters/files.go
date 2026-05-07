// Package adapters provides boundary adapters for external concerns like file I/O.
// All file read/write operations from core packages are consolidated here.
package adapters

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ===== Log File Operations (from ops/log.go) =====

// AppendRawLines appends raw bytes to a log file (for pre-formatted JSONL lines).
func AppendRawLines(logPath string, buf []byte) error {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("write to log %s: %w", logPath, err)
	}
	return nil
}

// ReadLog reads all lines from a log file as raw JSON lines.
func ReadLog(logPath string) ([][]byte, error) {
	return ReadLogFromOffset(logPath, 0)
}

// ReadLogFromOffset reads lines starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) ([][]byte, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }()

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
	return os.WriteFile(path, jsonData, 0644)
}

// LoadIssueJSON reads a JSON file and unmarshals it into the provided struct.
func LoadIssueJSON(path string, v any) error {
	data, err := os.ReadFile(path)
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
	return os.WriteFile(path, jsonData, 0644)
}

// LoadCheckpointJSON reads and unmarshals a checkpoint file.
func LoadCheckpointJSON(path string, v any) error {
	data, err := os.ReadFile(path)
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
	data, err := os.ReadFile(filePath)
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
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmpFile, err := os.CreateTemp(path, "manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()    //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("writing manifest temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("closing manifest temp file: %w", err)
	}

	dest := filepath.Join(path, "manifest.json")
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming manifest temp file: %w", err)
	}

	return nil
}

// WriteCacheFile writes raw bytes to a cache file named <id>.cache in path.
func WriteCacheFile(path string, id string, data []byte) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cacheFile := filepath.Join(path, id+".cache")
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	return nil
}

// ReadCacheFile reads the cache file named <id>.cache from path.
// If the file does not exist, it returns nil, nil.
func ReadCacheFile(path string, id string) ([]byte, error) {
	cacheFile := filepath.Join(path, id+".cache")
	data, err := os.ReadFile(cacheFile)
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
	return os.WriteFile(path, jsonData, 0644)
}

// LoadConfigFile reads and unmarshals a config file.
func LoadConfigFile(path string, v any) error {
	data, err := os.ReadFile(path)
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
	data, err := os.ReadFile(path)
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
	if err := os.WriteFile(tmp, jsonData, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadCoverageFile reads coverage data from a file.
// If the file does not exist, it returns nil, nil.
func ReadCoverageFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}
