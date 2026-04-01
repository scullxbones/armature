// Package exitcodes defines typed exit code constants for the trls CLI.
// All cmd error paths should use these constants rather than bare integers
// to ensure consistent exit codes across commands.
package exitcodes

// Code is a typed exit code for trls commands.
type Code int

const (
	// ExitSuccess indicates successful completion.
	ExitSuccess Code = 0

	// ExitGeneralError indicates an unexpected or unclassified error.
	ExitGeneralError Code = 1

	// ExitUsageError indicates incorrect CLI usage (bad flags, wrong arg count, etc.).
	ExitUsageError Code = 2

	// ExitNotFound indicates a requested resource (issue, file, etc.) does not exist.
	ExitNotFound Code = 3

	// ExitConflict indicates a conflict or already-exists condition
	// (e.g. claiming an already-claimed issue, duplicate issue ID).
	ExitConflict Code = 4

	// ExitIOError indicates a filesystem or network I/O failure.
	ExitIOError Code = 5

	// ExitInvalidState indicates the system or resource is in an unexpected state
	// (e.g. invalid status transition, broken dependency graph).
	ExitInvalidState Code = 6
)

// Int returns the integer value of the exit code, suitable for os.Exit.
func (c Code) Int() int {
	return int(c)
}

// String returns a short machine-readable label for the exit code.
// These labels are used in structured JSON error output.
func (c Code) String() string {
	switch c {
	case ExitSuccess:
		return "success"
	case ExitGeneralError:
		return "general_error"
	case ExitUsageError:
		return "usage_error"
	case ExitNotFound:
		return "not_found"
	case ExitConflict:
		return "conflict"
	case ExitIOError:
		return "io_error"
	case ExitInvalidState:
		return "invalid_state"
	default:
		return "unknown"
	}
}
