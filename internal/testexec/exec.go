// Package testexec is the only allowed source of execute-bit permission
// changes in tests.
//
// Write installs test executables without ETXTBSY races (golang/go#22315).
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

type fsDeps struct {
	mkdirTemp func(string, string) (string, error)
	writeFile func(string, []byte, os.FileMode) error
	remove    func(string) error
	link      func(string, string) error
	symlink   func(string, string) error
	lstat     func(string) (os.FileInfo, error)
	chmod     func(string, os.FileMode) error
}

func realDeps() fsDeps {
	return fsDeps{
		mkdirTemp: os.MkdirTemp,
		writeFile: os.WriteFile,
		remove:    os.Remove,
		link:      os.Link,
		symlink:   os.Symlink,
		lstat:     os.Lstat,
		chmod:     os.Chmod,
	}
}

type trampoline struct {
	mu      sync.Mutex
	ready   bool
	path    string
	prepErr error
	deps    fsDeps
}

func newTrampoline(deps fsDeps) *trampoline {
	if deps.mkdirTemp == nil {
		deps = realDeps()
	}
	return &trampoline{deps: deps}
}

func (tr *trampoline) prepare() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.ready {
		return tr.prepErr
	}
	dir, err := tr.deps.mkdirTemp("", "armature-testexec-")
	if err != nil {
		tr.prepErr = fmt.Errorf("mkdir trampoline: %w", err)
		tr.ready = true
		return tr.prepErr
	}
	path := filepath.Join(dir, "trampoline")
	if err := tr.deps.writeFile(path, []byte(trampolineBody), 0o755); err != nil {
		tr.prepErr = fmt.Errorf("write trampoline: %w", err)
		tr.ready = true
		return tr.prepErr
	}
	tr.path = path
	tr.ready = true
	return nil
}

// processTrampoline is the process-wide executable trampoline inode (ETXTBSY).
// Tests that need injected fs failures construct a local trampoline; they never
// reassign this pointer.
var processTrampoline = newTrampoline(realDeps()) //nolint:gochecknoglobals // process-wide trampoline inode prepared at init

func init() {
	abortIf(processTrampoline.prepare())
}

func abortIf(err error) {
	if err != nil {
		panic("testexec: " + err.Error())
	}
}

// Write materializes an executable at path by linking the process-wide
// trampoline and writing content to path+".script" (mode 0600). Callers must
// not os.WriteFile an executable in tests; use this helper instead.
func Write(t testing.TB, path string, content []byte) {
	t.Helper()
	writeWith(t, processTrampoline, path, content)
}

func writeWith(t testing.TB, tr *trampoline, path string, content []byte) {
	t.Helper()
	if err := tr.prepare(); err != nil {
		t.Fatalf("testexec.Write: %v", err)
	}
	tr.mu.Lock()
	tp := tr.path
	tr.mu.Unlock()
	if tp == "" {
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
	if err := tr.deps.remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("testexec.Write: remove %s: %v", path, err)
	}
	if err := tr.deps.link(tp, path); err != nil {
		if err := tr.deps.symlink(tp, path); err != nil {
			t.Fatalf("testexec.Write: link %s to trampoline: %v", path, err)
		}
	}
}

func ChmodDir(t testing.TB, dir string, mode os.FileMode) {
	t.Helper()
	chmodDirWith(t, processTrampoline.deps, dir, mode)
}

func chmodDirWith(t testing.TB, deps fsDeps, dir string, mode os.FileMode) {
	t.Helper()
	if deps.lstat == nil {
		deps = realDeps()
	}
	info, err := deps.lstat(dir)
	if err != nil {
		t.Fatalf("testexec.ChmodDir: stat %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("testexec.ChmodDir: %s is not a directory", dir)
	}
	if err := deps.chmod(dir, mode); err != nil {
		t.Fatalf("testexec.ChmodDir: chmod %s: %v", dir, err)
	}
}
