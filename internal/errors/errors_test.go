package errors

import (
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFailureCodeRegistryPanicsOnDuplicate_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Register of an already-registered code must panic")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, CodeGeneral1) && !strings.Contains(strings.ToLower(msg), "duplicate") {
			t.Errorf("panic = %v, want duplicate mention of %s", r, CodeGeneral1)
		}
	}()
	Register(CodeGeneral1)
}

func TestUnmappedErrorWrapsAsGeneral1_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	cause := stderrors.New("disk full")
	got := Unmapped(cause)
	if got == nil {
		t.Fatal("Unmapped returned nil")
	}
	if got.Code != CodeGeneral1 {
		t.Errorf("Code = %q, want %s", got.Code, CodeGeneral1)
	}
	if got.Cause != "disk full" {
		t.Errorf("Cause = %q, want disk full", got.Cause)
	}
	if got.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", got.ExitCode)
	}
	if got.NextActions == nil {
		t.Error("NextActions must be empty slice, not nil (JSON null)")
	}
	if len(got.NextActions) != 0 {
		t.Errorf("NextActions = %v, want empty (allowed on GENERAL-1)", got.NextActions)
	}
	if !stderrors.Is(got, cause) {
		t.Error("Unmapped must unwrap to the original error")
	}
	if got.Error() != "[GENERAL-1] disk full" {
		t.Errorf("Error() = %q, want [GENERAL-1] disk full", got.Error())
	}

	existing := New("USAGE", "bad flag", []string{"arm --help"}, 2)
	kept := Unmapped(existing)
	if kept.Code != "USAGE" {
		t.Errorf("Unmapped of CommandFailure Code = %q, want USAGE (must not re-wrap as GENERAL-1)", kept.Code)
	}

	if Unmapped(nil) != nil {
		t.Error("Unmapped(nil) must be nil")
	}
}

func TestArmatureErrorRemoved_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(file), "errors.go"))
	if err != nil {
		t.Fatalf("read errors.go: %v", err)
	}
	if strings.Contains(string(src), "ArmatureError") {
		t.Error("ArmatureError must be deleted from internal/errors")
	}
}

func TestPrefixFromModuleOrUse_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	if got := Prefix("claim"); got != "CLAIM" {
		t.Errorf("Prefix(claim) = %q, want CLAIM", got)
	}
	if got := Prefix("render-context"); got != "RENDER-CONTEXT" {
		t.Errorf("Prefix(render-context) = %q, want RENDER-CONTEXT", got)
	}
}
