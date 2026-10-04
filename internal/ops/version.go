package ops

import (
	"errors"
	"fmt"
)

// CurrentSchemaVersion is the ops JSONL schema version this binary writes and
// understands. Bump it only with a migration plan documented in
// docs/design/ops-schema-compatibility.md. Every newly written op record carries
// this value at positional array index 5.
const CurrentSchemaVersion = 1

// LegacySchemaVersion is assumed for op records written before schema_version
// was appended (5-element arrays). Those records are v1 by definition.
const LegacySchemaVersion = 1

// NewerSchemaVersionError is returned when a record's schema_version is newer
// than this binary understands. Callers must fail loud rather than skip the line.
type NewerSchemaVersionError struct {
	Seen      int
	Supported int
}

func (e *NewerSchemaVersionError) Error() string {
	return fmt.Sprintf(
		"ops schema version %d is newer than supported version %d; upgrade arm",
		e.Seen, e.Supported,
	)
}

// IsNewerSchemaVersion reports whether err is (or wraps) a newer-schema rejection.
func IsNewerSchemaVersion(err error) bool {
	var newer *NewerSchemaVersionError
	return errors.As(err, &newer)
}

// CheckSchemaVersion reports whether v is readable by this binary. Versions
// newer than CurrentSchemaVersion fail loudly so an older arm never silently
// mis-replays ops it cannot understand.
func CheckSchemaVersion(v int) error {
	if v < 1 {
		return fmt.Errorf("ops schema version %d is invalid: versions start at 1", v)
	}
	if v > CurrentSchemaVersion {
		return &NewerSchemaVersionError{Seen: v, Supported: CurrentSchemaVersion}
	}
	return nil
}

// EffectiveSchemaVersion returns the version to emit when marshaling. Zero
// (the Go zero value on Op) means "write current".
func EffectiveSchemaVersion(v int) int {
	if v == 0 {
		return CurrentSchemaVersion
	}
	return v
}
