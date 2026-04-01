package exitcodes_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/exitcodes"
)

func TestExitCodeValues(t *testing.T) {
	tests := []struct {
		name     string
		code     exitcodes.Code
		expected int
	}{
		{"ExitSuccess", exitcodes.ExitSuccess, 0},
		{"ExitGeneralError", exitcodes.ExitGeneralError, 1},
		{"ExitUsageError", exitcodes.ExitUsageError, 2},
		{"ExitNotFound", exitcodes.ExitNotFound, 3},
		{"ExitConflict", exitcodes.ExitConflict, 4},
		{"ExitIOError", exitcodes.ExitIOError, 5},
		{"ExitInvalidState", exitcodes.ExitInvalidState, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.code) != tt.expected {
				t.Errorf("expected %s = %d, got %d", tt.name, tt.expected, int(tt.code))
			}
		})
	}
}

func TestExitCodeDistinct(t *testing.T) {
	seen := map[int]exitcodes.Code{}
	all := []exitcodes.Code{
		exitcodes.ExitSuccess,
		exitcodes.ExitGeneralError,
		exitcodes.ExitUsageError,
		exitcodes.ExitNotFound,
		exitcodes.ExitConflict,
		exitcodes.ExitIOError,
		exitcodes.ExitInvalidState,
	}
	for _, code := range all {
		v := int(code)
		if prev, ok := seen[v]; ok {
			t.Errorf("duplicate exit code value %d used by %v and %v", v, prev, code)
		}
		seen[v] = code
	}
}

func TestExitCodeInt(t *testing.T) {
	// Verify Code.Int() returns the integer value.
	if exitcodes.ExitSuccess.Int() != 0 {
		t.Errorf("ExitSuccess.Int() expected 0, got %d", exitcodes.ExitSuccess.Int())
	}
	if exitcodes.ExitGeneralError.Int() != 1 {
		t.Errorf("ExitGeneralError.Int() expected 1, got %d", exitcodes.ExitGeneralError.Int())
	}
	if exitcodes.ExitNotFound.Int() != 3 {
		t.Errorf("ExitNotFound.Int() expected 3, got %d", exitcodes.ExitNotFound.Int())
	}
}

func TestExitCodeString(t *testing.T) {
	tests := []struct {
		code exitcodes.Code
		want string
	}{
		{exitcodes.ExitSuccess, "success"},
		{exitcodes.ExitGeneralError, "general_error"},
		{exitcodes.ExitUsageError, "usage_error"},
		{exitcodes.ExitNotFound, "not_found"},
		{exitcodes.ExitConflict, "conflict"},
		{exitcodes.ExitIOError, "io_error"},
		{exitcodes.ExitInvalidState, "invalid_state"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if tt.code.String() != tt.want {
				t.Errorf("expected String() = %q, got %q", tt.want, tt.code.String())
			}
		})
	}
}
