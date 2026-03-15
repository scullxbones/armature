package errors

import "fmt"

// TrellisError is a structured error with a code and optional context
type TrellisError struct {
	Code    string
	Message string
	Context map[string]string
	Cause   error
}

func (e *TrellisError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *TrellisError) Unwrap() error {
	return e.Cause
}

// NotFound returns a TrellisError indicating an issue was not found.
func NotFound(id string) *TrellisError {
	return &TrellisError{Code: "NOT_FOUND", Message: fmt.Sprintf("issue %s not found", id)}
}

// InvalidState returns a TrellisError indicating an invalid state.
func InvalidState(msg string) *TrellisError {
	return &TrellisError{Code: "INVALID_STATE", Message: msg}
}

// HookFailed returns a TrellisError indicating a hook failure.
func HookFailed(hook string, msg string) *TrellisError {
	return &TrellisError{Code: "HOOK_FAILED", Message: fmt.Sprintf("hook %s failed: %s", hook, msg)}
}

// IOError returns a TrellisError indicating an IO error with a cause.
func IOError(op string, cause error) *TrellisError {
	return &TrellisError{Code: "IO_ERROR", Message: fmt.Sprintf("IO error during %s", op), Cause: cause}
}
