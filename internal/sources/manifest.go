package sources

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const manifestFileName = "manifest.json"

// ReadManifest reads manifest.json from the given directory path.
// If the file does not exist, it returns an empty Manifest and no error.
func ReadManifest(path string) (Manifest, error) {
	filePath := filepath.Join(path, manifestFileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, nil
		}
		return Manifest{}, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := m.Unmarshal(data); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest: %w", err)
	}
	return m, nil
}

// WriteManifest marshals the manifest and writes it atomically to manifest.json
// in the given directory path.
func WriteManifest(path string, m Manifest) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmpFile, err := os.CreateTemp(path, "manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing manifest temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing manifest temp file: %w", err)
	}

	dest := filepath.Join(path, manifestFileName)
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming manifest temp file: %w", err)
	}

	return nil
}

// WriteCache writes raw bytes to a cache file named <id>.cache in path.
func WriteCache(path string, id string, data []byte) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cacheFile := filepath.Join(path, id+".cache")
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	return nil
}

// ReadCache reads the cache file named <id>.cache from path.
// If the file does not exist, it returns nil, nil.
func ReadCache(path string, id string) ([]byte, error) {
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
