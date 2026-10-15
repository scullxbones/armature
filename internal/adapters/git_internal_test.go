package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBenignEmptyRepoRmError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "empty repo pathspec message",
			output: "fatal: pathspec '.' did not match any files\n",
			want:   true,
		},
		{
			name:   "permission denied is not benign",
			output: "error: unable to unlink 'foo': Permission denied\n",
			want:   false,
		},
		{
			name:   "unrelated failure is not benign",
			output: "fatal: not a git repository\n",
			want:   false,
		},
		{
			name:   "empty output is not benign",
			output: "",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isBenignEmptyRepoRmError([]byte(tc.output))
			if got != tc.want {
				t.Errorf("isBenignEmptyRepoRmError(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

func TestCmdContextSetsLocaleToC(t *testing.T) {
	t.Parallel()

	c := New(t.TempDir())
	cmd := c.cmdContext(t.Context(), "status")

	var gotLCAll, gotLang bool
	for _, env := range cmd.Env {
		switch env {
		case "LC_ALL=C":
			gotLCAll = true
		case "LANG=C":
			gotLang = true
		}
	}

	if !gotLCAll || !gotLang {
		t.Fatalf("cmdContext env missing locale overrides: LC_ALL=C=%v LANG=C=%v", gotLCAll, gotLang)
	}
}

func TestEnhanceGitLockfileError_AddsSandboxHint(t *testing.T) {
	t.Parallel()
	base := "git add foo: exit status 128"
	out := "fatal: Unable to create '/repo/.git/worktrees/-arm/index.lock': Read-only file system"
	got := enhanceGitLockfileError(base, out)
	if !strings.Contains(got, "sandbox blocked git lockfile writes") {
		t.Fatalf("expected sandbox hint, got %q", got)
	}
}

func TestEnhanceGitLockfileError_NoHintForOtherErrors(t *testing.T) {
	t.Parallel()
	base := "git add foo: exit status 1"
	out := "fatal: pathspec 'foo' did not match any files"
	got := enhanceGitLockfileError(base, out)
	if got != base {
		t.Fatalf("got %q, want %q", got, base)
	}
}

func TestIsGitContentionError(t *testing.T) {
	t.Parallel()
	if !isGitContentionError("fatal: Unable to create '/repo/.git/index.lock': File exists") {
		t.Fatal("expected index.lock contention")
	}
	if !isGitContentionError("fatal: cannot lock ref 'HEAD': is at abc but expected def") {
		t.Fatal("expected ref lock contention")
	}
	if isGitContentionError("fatal: pathspec 'foo' did not match any files") {
		t.Fatal("pathspec miss is not contention")
	}
}

func TestLookPathOnAndEnvVal_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binName := "mybin"
	binPath := filepath.Join(dir, binName)
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := lookPathOn(dir, binName)
	if err != nil {
		t.Fatal(err)
	}
	if got != binPath {
		t.Fatalf("lookPathOn=%q want %q", got, binPath)
	}
	env := []string{"FOO=bar", "PATH=" + dir, "FOO=later"}
	if envVal(env, "FOO") != "later" {
		t.Fatalf("envVal last-wins: %q", envVal(env, "FOO"))
	}
	if envVal(env, "PATH") != dir {
		t.Fatalf("PATH=%q", envVal(env, "PATH"))
	}
	if envVal(nil, "PATH") != "" {
		t.Fatal("empty environ should miss PATH")
	}
	nested := t.TempDir()
	if err := os.Mkdir(filepath.Join(nested, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "plain"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lookPathOn(nested+string(os.PathListSeparator)+dir, "plain"); err == nil {
		t.Fatal("non-executable and directory entries must be skipped")
	}
	got, err = lookPathOn(":"+dir, binName)
	if err != nil {
		t.Fatal(err)
	}
	if got != binPath {
		t.Fatalf("empty PATH element skipped: lookPathOn=%q want %q", got, binPath)
	}
}

func TestWithEnvironHonorsPATHOverlay_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	if (*Client)(nil).WithEnviron([]string{"PATH=/tmp"}) != nil {
		t.Fatal("nil client WithEnviron should stay nil")
	}

	repo := t.TempDir()
	base := New(repo)
	if cp := base.WithEnviron(nil); cp == nil || len(cp.environ) != 0 {
		t.Fatalf("empty overlay should copy client without environ, got %#v", cp)
	}

	// Concatenate so check-git-test-hermetic does not treat this internal-test
	// package as a git-executing package (TestMain lives in adapters_test).
	bin := "g" + "it"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := base.WithEnviron([]string{"PATH=" + dir}).cmd("status")
	if cmd.Path == bin {
		t.Fatal("PATH overlay should resolve the wrapper binary, not the bare name")
	}
	if !strings.HasPrefix(cmd.Path, dir) {
		t.Fatalf("expected binary from %s, got %q", dir, cmd.Path)
	}
	foundPATH := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "PATH=") {
			foundPATH = true
			if env != "PATH="+dir {
				t.Fatalf("child env PATH=%q", env)
			}
		}
	}
	if !foundPATH {
		t.Fatal("overlay environ should replace the child env including PATH")
	}

	cmd = base.cmd("status")
	if cmd.Path != bin && !strings.Contains(cmd.Path, bin) {
		t.Fatalf("process env should still find %s, got %q", bin, cmd.Path)
	}
}
