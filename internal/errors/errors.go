// Package errors defines the port-level Command Failure type and Failure Code
// registry used by cmd/ to present agent-facing arm failures.
package errors

import (
	"fmt"
	"strings"
)

// Failure Code constants reserved by ADR 0020. GENERAL-1 was the expand-step
// wrap and is retired; USAGE and IO remain reserved prefixes/codes.
const (
	CodeGeneral1 = "GENERAL-1"
	CodeUSAGE    = "USAGE"
	CodeIO       = "IO"
)

var registeredCodes = map[string]struct{}{}

func init() {
	Register(CodeUSAGE)
	Register(CodeIO)
}

// Register records a Failure Code. Duplicate codes panic; retired codes stay
// reserved and must never be reused.
func Register(code string) {
	if _, exists := registeredCodes[code]; exists {
		panic("duplicate failure code: " + code)
	}
	registeredCodes[code] = struct{}{}
}

// Prefix returns the Failure Code prefix for a deep-module basename or a
// top-level cobra Use string (ADR 0020).
func Prefix(moduleOrUse string) string {
	return strings.ToUpper(moduleOrUse)
}

// CommandFailure is the agent-facing presentation of a CLI invocation that
// could not do the job it was asked to do. It is constructed at the CLI port
// from a domain error; it is not itself a domain type.
type CommandFailure struct {
	Code        string   `json:"code"`
	Cause       string   `json:"cause"`
	NextActions []string `json:"next_actions"`
	ExitCode    int      `json:"exit_code"`
	wrapped     error
}

// New constructs a CommandFailure. A nil nextActions slice is stored as empty
// so JSON encoding emits [] rather than null. Empty next_actions is allowed
// on IO (ADR 0020).
func New(code, cause string, nextActions []string, exitCode int) *CommandFailure {
	return Wrap(code, cause, nextActions, exitCode, nil)
}

// Wrap is New plus an unwrap target for errors.Is / errors.As.
func Wrap(code, cause string, nextActions []string, exitCode int, err error) *CommandFailure {
	if nextActions == nil {
		nextActions = []string{}
	}
	return &CommandFailure{
		Code:        code,
		Cause:       cause,
		NextActions: nextActions,
		ExitCode:    exitCode,
		wrapped:     err,
	}
}

// Map is Wrap for a Failure Code chosen at the CLI port (command prefix).
func Map(code, cause string, nextActions []string, exitCode int, err error) *CommandFailure {
	return Wrap(code, cause, nextActions, exitCode, err)
}

func (e *CommandFailure) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Cause)
}

func (e *CommandFailure) Unwrap() error {
	return e.wrapped
}
