package adapters_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

func TestCommitPathsNoVerify_ActionsBotWhenIdentMissing_REQ_481(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := append(identityFreeEnv(t), "GITHUB_ACTIONS=true")
	gitWithEnv(t, dir, env, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "SCHEMA"), []byte("schema\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithEnv(t, dir, env, "add", "SCHEMA")

	err := adapters.New(dir).WithEnviron(env).CommitPathsNoVerify("chore: refresh ops scaffolding", "SCHEMA")
	if err != nil {
		t.Fatalf("commit with no git ident on Actions: %v", err)
	}
	name, email := commitAuthor(t, dir, env)
	if name != "github-actions[bot]" || email != "41898282+github-actions[bot]@users.noreply.github.com" {
		t.Fatalf("author = %q <%s>", name, email)
	}
	if cn, ce := commitCommitter(t, dir, env); cn != name || ce != email {
		t.Fatalf("committer = %q <%s>", cn, ce)
	}
	cfg := exec.CommandContext(t.Context(), "git", "-C", dir, "config", "--local", "--get", "user.name")
	cfg.Env = env
	if out, cfgErr := cfg.CombinedOutput(); cfgErr == nil {
		t.Fatalf("bot identity must not be written to repo config, got %q", strings.TrimSpace(string(out)))
	}
}

func TestCommitPathsNoVerify_KeepsConfiguredIdentOnActions_REQ_481(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := append(identityFreeEnv(t), "GITHUB_ACTIONS=true")
	gitWithEnv(t, dir, env, "init", "-b", "main")
	gitWithEnv(t, dir, env, "config", "user.name", "Ada Lovelace")
	gitWithEnv(t, dir, env, "config", "user.email", "ada@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SCHEMA"), []byte("schema\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithEnv(t, dir, env, "add", "SCHEMA")

	if err := adapters.New(dir).WithEnviron(env).CommitPathsNoVerify("chore: refresh ops scaffolding", "SCHEMA"); err != nil {
		t.Fatal(err)
	}
	name, email := commitAuthor(t, dir, env)
	if name != "Ada Lovelace" || email != "ada@example.com" {
		t.Fatalf("configured ident overridden: %q <%s>", name, email)
	}
}

func TestCommitPathsNoVerify_MissingIdentOutsideActions_REQ_481(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := identityFreeEnv(t)
	gitWithEnv(t, dir, env, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "SCHEMA"), []byte("schema\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitWithEnv(t, dir, env, "add", "SCHEMA")

	err := adapters.New(dir).WithEnviron(env).CommitPathsNoVerify("chore: refresh ops scaffolding", "SCHEMA")
	if err == nil || !identityRejected(err.Error()) {
		t.Fatalf("expected missing-ident failure outside Actions, got %v", err)
	}
}

func identityRejected(msg string) bool {
	return strings.Contains(msg, "Author identity unknown") ||
		strings.Contains(msg, "empty ident") ||
		strings.Contains(msg, "unable to auto-detect email")
}

func identityFreeEnv(t *testing.T) []string {
	t.Helper()
	blank := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(blank, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	drop := map[string]bool{
		"GIT_AUTHOR_NAME":     true,
		"GIT_AUTHOR_EMAIL":    true,
		"GIT_COMMITTER_NAME":  true,
		"GIT_COMMITTER_EMAIL": true,
		"GIT_CONFIG_GLOBAL":   true,
		"GIT_CONFIG_SYSTEM":   true,
		"EMAIL":               true,
	}
	out := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		name, _, ok := strings.Cut(e, "=")
		if !ok || drop[name] || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "GIT_CONFIG_GLOBAL="+blank, "GIT_CONFIG_NOSYSTEM=1")
}

func gitWithEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func commitAuthor(t *testing.T, dir string, env []string) (string, string) {
	t.Helper()
	return commitIdent(t, dir, env, "%an", "%ae")
}

func commitCommitter(t *testing.T, dir string, env []string) (string, string) {
	t.Helper()
	return commitIdent(t, dir, env, "%cn", "%ce")
}

func commitIdent(t *testing.T, dir string, env []string, nameFmt, emailFmt string) (string, string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "-C", dir, "log", "-1", "--format="+nameFmt+"%x00"+emailFmt)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	parts := strings.Split(strings.TrimRight(string(out), "\n"), "\x00")
	if len(parts) != 2 {
		t.Fatalf("ident output %q", out)
	}
	return parts[0], parts[1]
}
