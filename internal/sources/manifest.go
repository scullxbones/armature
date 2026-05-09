package sources

import (
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
)

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
