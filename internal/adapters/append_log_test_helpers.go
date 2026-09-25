package adapters

import "path/filepath"

func SimulatePendingMarker(logPath string, start int64, data []byte) error {
	metaDir, err := appendMetaDir(logPath)
	if err != nil {
		return err
	}
	metaBase := filepath.Join(metaDir, filepath.Base(logPath))
	markerPath := metaBase + pendingMarkerSuffix
	return writePendingMarker(markerPath, pendingAppend{Start: start, Data: data})
}
