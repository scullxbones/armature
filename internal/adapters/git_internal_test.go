package adapters

import (
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
