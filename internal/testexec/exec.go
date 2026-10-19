// Package testexec installs test executables without ETXTBSY races (golang/go#22315).
//
// A process that forks while any goroutine holds a write fd to an executable
// can fail exec of that inode with ETXTBSY: O_CLOEXEC closes the fd only at
// exec, so the child still has it open. Rename-into-place does not close the
// inherited fd. Tests therefore never create a new executable inode after
// TestMain: Write hard-links a trampoline prepared in init and stores unique
// script bodies in a non-executable sidecar.
package testexec

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const trampolineBody = `#!/bin/sh
resolve() {
  n=$1
  case "$n" in
    /*) printf '%s\n' "$n"; return ;;
  esac
  oldifs=$IFS
  IFS=:
  for d in $PATH; do
    IFS=$oldifs
    if [ -n "$d" ] && [ -x "$d/$n" ] && [ -f "$d/$n.script" ]; then
      printf '%s\n' "$d/$n"
      return
    fi
  done
  IFS=$oldifs
  printf '%s\n' "$n"
}
exe=$(resolve "$0")
script=$exe.script
if [ ! -f "$script" ]; then
  echo "testexec: missing sidecar $script (argv0=$0)" >&2
  exit 127
fi
exec /bin/sh "$script" "$@"
`

var (
	prepareMu      sync.Mutex
	prepared       bool
	trampolinePath string
	prepareErr     error

	mkdirTemp = os.MkdirTemp
	writeFile = os.WriteFile
	remove    = os.Remove
	link      = os.Link
	symlink   = os.Symlink
)

func init() {
	abortIf(prepare())
}

func abortIf(err error) {
	if err != nil {
		panic("testexec: " + err.Error())
	}
}

func prepare() error {
	prepareMu.Lock()
	defer prepareMu.Unlock()
	if prepared {
		return prepareErr
	}
	dir, err := mkdirTemp("", "armature-testexec-")
	if err != nil {
		prepareErr = fmt.Errorf("mkdir trampoline: %w", err)
		prepared = true
		return prepareErr
	}
	path := filepath.Join(dir, "trampoline")
	if err := writeFile(path, []byte(trampolineBody), 0o755); err != nil {
		prepareErr = fmt.Errorf("write trampoline: %w", err)
		prepared = true
		return prepareErr
	}
	trampolinePath = path
	prepared = true
	return nil
}

// Write materializes an executable at path by linking the process-wide
// trampoline and writing content to path+".script" (mode 0600). Callers must
// not os.WriteFile an executable in tests; use this helper instead.
func Write(t testing.TB, path string, content []byte) {
	t.Helper()
	if err := prepare(); err != nil {
		t.Fatalf("testexec.Write: %v", err)
	}
	if trampolinePath == "" {
		t.Fatal("testexec.Write: trampoline was not prepared before tests ran")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("testexec.Write: mkdir %s: %v", dir, err)
		}
	}
	sidecar := path + ".script"
	if err := os.WriteFile(sidecar, content, 0o600); err != nil {
		t.Fatalf("testexec.Write: sidecar %s: %v", sidecar, err)
	}
	if err := remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("testexec.Write: remove %s: %v", path, err)
	}
	if err := link(trampolinePath, path); err != nil {
		if err := symlink(trampolinePath, path); err != nil {
			t.Fatalf("testexec.Write: link %s to trampoline: %v", path, err)
		}
	}
}
