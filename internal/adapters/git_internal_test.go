package adapters

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/testexec"
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
	testexec.Write(t, binPath, []byte("#!/bin/sh\nexit 0\n"))
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
	if envValFold([]string{"Path=/win/bin", "PATH=/unix/bin"}, "PATH") != "/unix/bin" {
		t.Fatal("fold last-wins among Path/PATH")
	}
	if envValFold([]string{"path=/msys"}, "PATH") != "/msys" {
		t.Fatal("fold matches PATH key case-insensitively")
	}
	if envVal([]string{"Path=/win/bin"}, "PATH") != "" {
		t.Fatal("exact PATH lookup must not match Path")
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
	got, err = lookPathOn(string(os.PathListSeparator)+dir, binName)
	if err != nil {
		t.Fatal(err)
	}
	if got != binPath {
		t.Fatalf("empty PATH element is cwd then later abs: lookPathOn=%q want %q", got, binPath)
	}
}

func TestLookPathOnRejectsRelativeEntry_REQ_PARCLI(t *testing.T) { //nolint:paralleltest // t.Chdir is process-wide
	t.Chdir(t.TempDir())
	if err := os.Mkdir("rel", 0o755); err != nil {
		t.Fatal(err)
	}
	testexec.Write(t, filepath.Join("rel", "mybin"), []byte("#!/bin/sh\nexit 0\n"))
	got, err := lookPathOn("rel", "mybin")
	if got == "" {
		t.Fatal("LookPath returns the relative path together with ErrDot")
	}
	if !errors.Is(err, exec.ErrDot) {
		t.Fatalf("err=%v want ErrDot", err)
	}
}

func TestLookPathOnWindowsPATHEXT_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(dir, "mybin.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := lookPathOnConfig(dir, "mybin", lookPathConfig{
		windows: true,
		pathext: ".COM;.EXE;.BAT",
		listSep: os.PathListSeparator,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != exe {
		t.Fatalf("lookPathOnConfig=%q want %q (no exec bits, PATHEXT)", got, exe)
	}
	_, err = lookPathOnConfig(dir, "missing", lookPathConfig{windows: true, pathext: ".EXE"})
	if err == nil {
		t.Fatal("missing name must not resolve")
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

	// The git binary name is an allowlisted PATH-overlay fixture
	// (scripts/repoguard allow git-hermetic-name); TestMain lives in adapters_test.
	bin := "git"
	dir := t.TempDir()
	testexec.Write(t, filepath.Join(dir, bin), []byte("#!/bin/sh\nexit 1\n"))
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

func TestAbsolutizeLocalRemoteURL_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	cases := []struct {
		name string
		url  string
		want func(t *testing.T, got string)
	}{
		{
			name: "relative_dotdot",
			url:  "../o.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				exp, err := filepath.Abs(filepath.Join(base, "../o.git"))
				if err != nil {
					t.Fatal(err)
				}
				if got != exp {
					t.Fatalf("got %q want %q", got, exp)
				}
			},
		},
		{
			name: "relative_dot",
			url:  "./origin.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				exp, err := filepath.Abs(filepath.Join(base, "./origin.git"))
				if err != nil {
					t.Fatal(err)
				}
				if got != exp {
					t.Fatalf("got %q want %q", got, exp)
				}
			},
		},
		{
			name: "https_unchanged",
			url:  "https://example.com/org/repo.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				if got != "https://example.com/org/repo.git" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "scp_like_unchanged",
			url:  "git@github.com:org/repo.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				if got != "git@github.com:org/repo.git" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "file_scheme_unchanged",
			url:  "file:///tmp/o.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				if got != "file:///tmp/o.git" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "absolute_path_unchanged",
			url:  "/abs/o.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				if got != "/abs/o.git" {
					t.Fatalf("got %q", got)
				}
			},
		},
		{
			name: "path_with_colon_after_slash_relative",
			url:  "remotes/host:path.git",
			want: func(t *testing.T, got string) {
				t.Helper()
				// slash before colon → not scp-like; treat as relative local path
				exp, err := filepath.Abs(filepath.Join(base, "remotes/host:path.git"))
				if err != nil {
					t.Fatal(err)
				}
				if got != exp {
					t.Fatalf("got %q want %q", got, exp)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := absolutizeLocalRemoteURL(base, tc.url)
			if err != nil {
				t.Fatalf("absolutize: %v", err)
			}
			tc.want(t, got)
		})
	}

	if _, err := absolutizeLocalRemoteURL(base, "  "); err == nil {
		t.Fatal("empty URL must error")
	}
}

func TestIsSCPLikeRemoteURL_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want bool
	}{
		{"git@github.com:org/repo.git", true},
		{"github.com:org/repo.git", true},
		{"https://example.com/r.git", false},
		{"/abs/o.git", false},
		{"./rel.git", false},
		{"[::1]:repo.git", false},
		{`C:\o.git`, false},
		{"remotes/host:path", false},
		{"nocolon", false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			if got := isSCPLikeRemoteURL(tc.url); got != tc.want {
				t.Fatalf("isSCPLikeRemoteURL(%q)=%v want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestLocalTransportConfigEmpty_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	// No git repo → config fails (not the empty-match exit 1 path).
	c := New(repo)
	if _, err := c.localTransportConfig(); err == nil {
		t.Fatal("expected error for non-repo")
	}
}

func TestNetworkCmdHonorsTimeout_REQ_TOPTIER_S12_T1G(t *testing.T) { //nolint:paralleltest // mutates gitNetworkTimeout
	// Shrink shared deadline; restore after. Not parallel — mutates package var.
	prev := gitNetworkTimeout
	gitNetworkTimeout = 100 * time.Millisecond
	defer func() { gitNetworkTimeout = prev }()

	wrapperDir := t.TempDir()
	// Use /bin/sleep: PATH overlay is wrapper-only, so bare `sleep` is missing
	// and the arm falls through to exit 0 without stalling.
	testexec.Write(t, filepath.Join(wrapperDir, "git"), []byte(`#!/bin/sh
for a in "$@"; do
  case "$a" in
    # exec so CommandContext kills the sleeper itself (not an orphaned child).
    ls-remote|fetch) exec /bin/sleep 30 ;;
  esac
done
exit 0
`))
	repo := t.TempDir()
	c := New(repo).WithEnviron([]string{"PATH=" + wrapperDir})
	start := time.Now()
	cmd, cancel := c.networkCmd("ls-remote", "--refs", "origin", "refs/heads/_armature")
	defer cancel()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error from stalled ls-remote")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("networkCmd took %v; want fail-closed near %v", elapsed, gitNetworkTimeout)
	}
}
