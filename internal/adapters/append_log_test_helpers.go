package adapters

import "path/filepath"

// SimulatePendingMarker writes a .pending marker for logPath as if a prior
// AppendLog.Append call crashed after making the marker durable but before
// completing the write it describes (start offset and full record bytes).
// The next AppendLog.Append call against logPath will recover from this
// marker: replaying/completing the write if it never fully landed, or
// recognizing it as already-complete and clearing the marker if it did.
func SimulatePendingMarker(logPath string, start int64, data []byte) error {
	metaDir, err := appendMetaDir(logPath)
	if err != nil {
		return err
	}
	metaBase := filepath.Join(metaDir, filepath.Base(logPath))
	markerPath := metaBase + pendingMarkerSuffix
	return writePendingMarker(markerPath, pendingAppend{Start: start, Data: data})
}
