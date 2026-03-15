package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

func TestTrellisError_Error(t *testing.T) {
	tests := []struct {
		name        string
		err         *TrellisError
		wantCode    string
		wantMessage string
	}{
		{
			name:        "NotFound",
			err:         NotFound("ISS-42"),
			wantCode:    "NOT_FOUND",
			wantMessage: "ISS-42 not found",
		},
		{
			name:        "InvalidState",
			err:         InvalidState("cannot transition from closed"),
			wantCode:    "INVALID_STATE",
			wantMessage: "cannot transition from closed",
		},
		{
			name:        "HookFailed",
			err:         HookFailed("pre-check", "exit code 1"),
			wantCode:    "HOOK_FAILED",
			wantMessage: "pre-check",
		},
		{
			name:        "IOError",
			err:         IOError("read file", stderrors.New("no such file")),
			wantCode:    "IO_ERROR",
			wantMessage: "read file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.err.Error()
			if !strings.Contains(s, tt.wantCode) {
				t.Errorf("Error() = %q, want to contain code %q", s, tt.wantCode)
			}
			if !strings.Contains(s, tt.wantMessage) {
				t.Errorf("Error() = %q, want to contain message %q", s, tt.wantMessage)
			}
		})
	}
}

func TestTrellisError_Unwrap(t *testing.T) {
	cause := stderrors.New("underlying io error")
	err := IOError("write", cause)

	if !stderrors.Is(err, cause) {
		t.Error("errors.Is should find cause through Unwrap")
	}

	var trellisErr *TrellisError
	if !stderrors.As(err, &trellisErr) {
		t.Error("errors.As should find TrellisError")
	}
	if trellisErr.Code != "IO_ERROR" {
		t.Errorf("expected code IO_ERROR, got %s", trellisErr.Code)
	}
}
