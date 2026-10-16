package ops

import (
	"fmt"
	"path/filepath"
)

// ParseLoadError locates a parse failure at a 1-based physical line in a log.
// Newer-schema failures unwrap to NewerSchemaVersionError. Other corrupt lines
// carry a quarantine repair command.
type ParseLoadError struct {
	LogPath string
	Line    int
	Err     error
}

func (e *ParseLoadError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d: %v", e.LogPath, e.Line, e.Err)
}

func (e *ParseLoadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RepairCommand is the single arm invocation that quarantines this corrupt line.
// Newer-schema lines are not repairable this way; upgrade arm instead.
func (e *ParseLoadError) RepairCommand() string {
	if e == nil || isNewerSchemaVersion(e.Err) {
		return ""
	}
	return fmt.Sprintf("arm doctor quarantine --log %s --line %d", filepath.Base(e.LogPath), e.Line)
}

func locateParseError(logPath string, line int, err error) error {
	return &ParseLoadError{LogPath: logPath, Line: line, Err: err}
}
