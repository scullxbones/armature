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
		if !strings.Contains(msg, CodeUSAGE) && !strings.Contains(strings.ToLower(msg), "duplicate") {
			t.Errorf("panic = %v, want duplicate mention of %s", r, CodeUSAGE)
		}
	}()
	Register(CodeUSAGE)
}

func TestUnmappedRemoved_REQ_LNGHZN_S6_T5(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(file), "errors.go"))
	if err != nil {
		t.Fatalf("read errors.go: %v", err)
	}
	if strings.Contains(string(src), "func Unmapped") {
		t.Error("Unmapped must be deleted; GENERAL-1 wrap is retired")
	}
	if _, registered := registeredCodes[CodeGeneral1]; registered {
		t.Error("GENERAL-1 must not remain in the live registry after wrap deletion")
	}
}

func TestCommandFailurePreservesUnwrap(t *testing.T) {
	t.Parallel()
	cause := stderrors.New("disk full")
	got := Wrap(CodeIO, "disk full", nil, 1, cause)
	if !stderrors.Is(got, cause) {
		t.Error("Wrap must unwrap to the original error")
	}
	if got.Error() != "[IO] disk full" {
		t.Errorf("Error() = %q, want [IO] disk full", got.Error())
	}

	existing := New("USAGE", "bad flag", []string{"arm --help"}, 2)
	if existing.Code != "USAGE" {
		t.Errorf("Code = %q, want USAGE", existing.Code)
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
