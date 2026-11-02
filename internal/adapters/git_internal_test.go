package adapters

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/gitoid"
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

func TestParseCommitWalkOutput_KeepsFirstParentMergeBlock_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	// Synthetic `git log -z --diff-merges=separate --name-only
	// --format=%H%x09%ct%x09%P` shape: merge SHA repeated once per parent;
	// first path field has a leading LF.
	const merge = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const p1 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const p2 = "cccccccccccccccccccccccccccccccccccccccc"
	const next = "dddddddddddddddddddddddddddddddddddddddd"
	raw := merge + "\t100\t" + p1 + " " + p2 + "\x00" +
		"\nfrom-first-parent.txt\x00" +
		merge + "\t100\t" + p1 + " " + p2 + "\x00" +
		"\nfrom-second-parent.txt\x00" +
		next + "\t200\t" + merge + "\x00"

	got, err := parseCommitWalkOutput([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries=%d want 2 (merge once + follower)", len(got))
	}
	if got[0].SHA != merge {
		t.Fatalf("first sha=%s want merge", got[0].SHA)
	}
	if got[0].FirstParent != p1 {
		t.Fatalf("first parent=%s want %s", got[0].FirstParent, p1)
	}
	if len(got[0].ChangedFiles) != 1 || got[0].ChangedFiles[0] != "from-first-parent.txt" {
		t.Fatalf("merge files=%v want [from-first-parent.txt]", got[0].ChangedFiles)
	}
	if got[1].SHA != next {
		t.Fatalf("second sha=%s", got[1].SHA)
	}
}

func TestParseCommitWalkOutput_SHA256ObjectIDs_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	const merge = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const p1 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const p2 = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if len(merge) != gitoid.LenSHA256 || len(p1) != gitoid.LenSHA256 {
		t.Fatal("test fixtures must be 64-hex")
	}
	raw := merge + "\t100\t" + p1 + " " + p2 + "\x00" +
		"\nops/worker.log\x00" +
		merge + "\t100\t" + p1 + " " + p2 + "\x00" +
		"\nops/other.log\x00"
	got, err := parseCommitWalkOutput([]byte(raw))
	if err != nil {
		t.Fatalf("parse sha256 walk: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries=%d want 1", len(got))
	}
	if got[0].SHA != merge || got[0].FirstParent != p1 {
		t.Fatalf("got sha=%s parent=%s", got[0].SHA, got[0].FirstParent)
	}
	if len(got[0].ChangedFiles) != 1 || got[0].ChangedFiles[0] != "ops/worker.log" {
		t.Fatalf("files=%v", got[0].ChangedFiles)
	}
}

func TestIsGitObjectID_SHA1AndSHA256_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	if !isGitObjectID(strings.Repeat("a", gitoid.LenSHA1)) {
		t.Fatal("40-hex lowercase must be accepted")
	}
	if !isGitObjectID(strings.Repeat("b", gitoid.LenSHA256)) {
		t.Fatal("64-hex lowercase must be accepted")
	}
	bads := []string{
		"", "abc",
		strings.Repeat("0", gitoid.LenSHA1-1), strings.Repeat("0", gitoid.LenSHA1+1),
		strings.Repeat("0", gitoid.LenSHA256-1), strings.Repeat("0", gitoid.LenSHA256+1),
		strings.Repeat("g", gitoid.LenSHA1),
		strings.Repeat("A", gitoid.LenSHA1), // schema Exact: lowercase only
	}
	for _, bad := range bads {
		if isGitObjectID(bad) {
			t.Fatalf("accepted invalid oid %q", bad)
		}
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

func TestNetworkCmdHonorsTimeout_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	timeout := 100 * time.Millisecond

	wrapperDir := t.TempDir()
	// Child sleeper under a process group: Cancel must kill the group, not leave
	// an orphaned git-remote-http stand-in hanging CombinedOutput.
	testexec.Write(t, filepath.Join(wrapperDir, "git"), []byte(`#!/bin/sh
for a in "$@"; do
  case "$a" in
    ls-remote|fetch)
      /bin/sleep 30 &
      child=$!
      # Wait on child so the parent git wrapper stays alive until group kill.
      wait "$child"
      exit 0
      ;;
  esac
done
exit 0
`))
	repo := t.TempDir()
	c := New(repo).WithEnviron([]string{"PATH=" + wrapperDir}).WithNetworkTimeout(timeout)
	start := time.Now()
	cmd, ctx, cancel := c.networkCmd("ls-remote", "--refs", "origin", "refs/heads/_armature")
	defer cancel()
	err := cmd.Run()
	if ctx.Err() != nil {
		err = errors.Join(ctx.Err(), err)
	}
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	// timeout + WaitDelay slack
	if elapsed > 3*time.Second {
		t.Fatalf("networkCmd took %v; want fail-closed near %v (+ WaitDelay)", elapsed, timeout)
	}
}

func TestParseRunLeaseAndPidAlive_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	pid, started, ok := parseRunLease("pid=123\nstarted=99\n")
	if !ok || pid != 123 || started != 99 {
		t.Fatalf("parseRunLease got pid=%d started=%d ok=%v", pid, started, ok)
	}
	if _, _, ok := parseRunLease("pid=nope\n"); ok {
		t.Fatal("expected invalid pid to fail")
	}
	if _, _, ok := parseRunLease("pid=1\nstarted=nope\n"); ok {
		t.Fatal("expected bad started to fail")
	}
	if _, _, ok := parseRunLease(""); ok {
		t.Fatal("expected empty lease to fail")
	}
	if pidAlive(0) || pidAlive(-1) {
		t.Fatal("pidAlive must reject non-positive pids")
	}
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive must see current process")
	}
	if leaseKindForRefNS(armatureTmpNS) != leaseKindArmatureTmp {
		t.Fatal("expected exact armature-tmp lease kind")
	}
	if leaseKindForRefNS(armatureTmpNS+"/x") != leaseKindArmatureTmp {
		t.Fatal("expected armature-tmp lease kind")
	}
	if leaseKindForRefNS("refs/remotes/origin/_armature-run") != leaseKindOriginRun {
		t.Fatal("expected origin-run lease kind")
	}
}

func TestParseRunLeaseStartedOptional_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	// pid alone is enough for ok (started defaults 0); age backstop then uses mtime.
	pid, started, ok := parseRunLease("pid=7\n")
	if !ok || pid != 7 || started != 0 {
		t.Fatalf("got pid=%d started=%d ok=%v", pid, started, ok)
	}
}

func TestPidAlive_EPERMTreatedAlive_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	// pid 1 exists; Signal(0) returns EPERM for unprivileged callers — must be alive.
	if !pidAlive(1) {
		t.Fatal("pidAlive(1) must be true (EPERM means process exists)")
	}
}

func TestExactRefFromPatternOutput_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()
	want := "refs/heads/_armature"
	sha, found, err := exactRefFromPatternOutput(
		"abc123 refs/foo/refs/heads/_armature\ndef456 refs/heads/_armature\n", want)
	if err != nil || !found || sha != "def456" {
		t.Fatalf("exact hit got sha=%q found=%v err=%v", sha, found, err)
	}
	sha, found, err = exactRefFromPatternOutput("abc123 refs/foo/refs/heads/_armature\n", want)
	if err != nil || found || sha != "" {
		t.Fatalf("suffix-only must miss: sha=%q found=%v err=%v", sha, found, err)
	}
	_, found, err = exactRefFromPatternOutput("refs/heads/_armature\n", want)
	if err != nil || !found {
		t.Fatalf("refname-only format: found=%v err=%v", found, err)
	}
	refs, err := refsUnderPrefix(
		"refs/armature-tmp/dead/bundle\nrefs/other/armature-tmp/dead\nrefs/armature-tmp/dead\n",
		"refs/armature-tmp/dead")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0] != "refs/armature-tmp/dead/bundle" || refs[1] != "refs/armature-tmp/dead" {
		t.Fatalf("refsUnderPrefix got %v", refs)
	}
}
