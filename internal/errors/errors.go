package errors

import "fmt"

// ArmatureError is a structured error with a code and optional context
type ArmatureError struct {
	Code    string
	Message string
	Context map[string]string
	Cause   error
}

func (e *ArmatureError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *ArmatureError) Unwrap() error {
	return e.Cause
}

// NotFound returns an ArmatureError indicating an issue was not found.
func NotFound(id string) *ArmatureError {
	return &ArmatureError{Code: "NOT_FOUND", Message: fmt.Sprintf("issue %s not found", id)}
}

// InvalidState returns an ArmatureError indicating an invalid state.
func InvalidState(msg string) *ArmatureError {
	return &ArmatureError{Code: "INVALID_STATE", Message: msg}
}

// HookFailed returns an ArmatureError indicating a hook failure.
func HookFailed(hook string, msg string) *ArmatureError {
	return &ArmatureError{Code: "HOOK_FAILED", Message: fmt.Sprintf("hook %s failed: %s", hook, msg)}
}

// IOError returns an ArmatureError indicating an IO error with a cause.
func IOError(op string, cause error) *ArmatureError {
	return &ArmatureError{Code: "IO_ERROR", Message: fmt.Sprintf("IO error during %s", op), Cause: cause}
}
