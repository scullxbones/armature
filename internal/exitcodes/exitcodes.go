// Package exitcodes defines typed exit code constants for the arm CLI.
// All cmd error paths should use these constants rather than bare integers
// to ensure consistent exit codes across commands.
package exitcodes

// Code is a typed exit code for the arm CLI.
type Code int

const (
	// ExitSuccess indicates successful completion.
	ExitSuccess Code = 0

	// ExitGeneralError indicates an unexpected or unclassified error.
	// This is the process exit for every live Failure Code except USAGE,
	// including reserved IO (docs/error-contract.md).
	ExitGeneralError Code = 1

	// ExitUsageError indicates incorrect CLI usage (bad flags, wrong arg count, etc.).
	// This is the process exit for Failure Code USAGE (docs/error-contract.md).
	ExitUsageError Code = 2

	// ExitNotFound is reserved by docs/design/cli-grammar-contract.md
	// (typed set: success/general/usage/not-found/conflict/io/invalid-state).
	ExitNotFound Code = 3

	// ExitConflict is reserved by docs/design/cli-grammar-contract.md.
	ExitConflict Code = 4

	// ExitIOError is reserved by docs/design/cli-grammar-contract.md.
	// It is not the Command Failure mapping for code IO (that exit is 1).
	ExitIOError Code = 5

	// ExitInvalidState is reserved by docs/design/cli-grammar-contract.md.
	ExitInvalidState Code = 6
)

// Int returns the integer value of the exit code, suitable for os.Exit.
func (c Code) Int() int {
	return int(c)
}
