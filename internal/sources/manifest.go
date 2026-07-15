package sources

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

// FileCommitter is an interface for committing file changes (manifest and cache).
type FileCommitter interface {
	CommitWorktreeOp(relPath, message string) error
}

// ReadManifest reads manifest.json from the given directory path.
// If the file does not exist, it returns an empty Manifest and no error.
func ReadManifest(path string) (Manifest, error) {
	data, err := adapters.ReadManifestFile(path)
	if err != nil {
		return Manifest{}, err
	}
	if data == nil {
		return Manifest{}, nil
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
	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	return adapters.WriteManifestFile(path, data)
}

// WriteCache writes raw bytes to a cache file named <id>.cache in path.
func WriteCache(path string, id string, data []byte) error {
	return adapters.WriteCacheFile(path, id, data)
}

// ReadCache reads the cache file named <id>.cache from path.
// If the file does not exist, it returns nil, nil.
func ReadCache(path string, id string) ([]byte, error) {
	return adapters.ReadCacheFile(path, id)
}

// WriteManifestAndCommit writes the manifest to manifest.json and commits it
// to the worktree's _armature branch if worktreePath is non-empty.
// Pass worktreePath="" to skip the commit (single-branch mode).
func WriteManifestAndCommit(manifestPath, worktreePath string, m Manifest, fc FileCommitter) error {
	if err := WriteManifest(manifestPath, m); err != nil {
		return err
	}
	if worktreePath == "" {
		return nil // single-branch: no git commit needed
	}

	// Compute the relative path from worktreePath to manifest.json
	manifestFile := filepath.Join(manifestPath, "manifest.json")
	relPath, err := filepath.Rel(worktreePath, manifestFile)
	if err != nil {
		return fmt.Errorf("resolve relative manifest path: %w", err)
	}

	return fc.CommitWorktreeOp(relPath, "sources: update manifest.json")
}

// WriteCacheAndCommit writes cache data to a cache file and commits it
// to the worktree's _armature branch if worktreePath is non-empty.
// Pass worktreePath="" to skip the commit (single-branch mode).
func WriteCacheAndCommit(manifestPath, worktreePath, id string, data []byte, fc FileCommitter) error {
	if err := WriteCache(manifestPath, id, data); err != nil {
		return err
	}
	if worktreePath == "" {
		return nil // single-branch: no git commit needed
	}

	// Compute the relative path from worktreePath to the cache file
	cacheFile := filepath.Join(manifestPath, id+".cache")
	relPath, err := filepath.Rel(worktreePath, cacheFile)
	if err != nil {
		return fmt.Errorf("resolve relative cache path: %w", err)
	}

	message := fmt.Sprintf("sources: update cache for %s", id)
	return fc.CommitWorktreeOp(relPath, message)
}
