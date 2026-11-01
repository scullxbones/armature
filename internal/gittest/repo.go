package gittest

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

type Fixture struct {
	Dir    string
	Origin string
}

type seedCache struct {
	once         sync.Once
	err          error
	worktreeSeed string
	originSeed   string
}

// seeds holds process-wide disposable git fixture trees (copied per test).
var seeds = &seedCache{} //nolint:gochecknoglobals // process-wide fixture seed trees; never reassigned

func Init(t *testing.T) string {
	t.Helper()
	if err := IsolateGit(); err != nil {
		t.Fatalf("IsolateGit: %v", err)
	}
	if err := ensureSeeds(); err != nil {
		t.Fatalf("ensureSeeds: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(seeds.worktreeSeed, dir); err != nil {
		t.Fatalf("copy worktree seed: %v", err)
	}
	return dir
}

func InitRepo(t *testing.T) string {
	t.Helper()
	return Init(t)
}

func InitWithOrigin(t *testing.T) Fixture {
	t.Helper()
	dir := Init(t)
	originParent := t.TempDir()
	origin := filepath.Join(originParent, "origin.git")
	if err := os.MkdirAll(origin, 0o750); err != nil {
		t.Fatalf("mkdir origin: %v", err)
	}
	if err := copyTree(seeds.originSeed, origin); err != nil {
		t.Fatalf("copy origin seed: %v", err)
	}
	if err := appendOriginRemote(dir, origin); err != nil {
		t.Fatalf("attach origin: %v", err)
	}
	return Fixture{Dir: dir, Origin: origin}
}

func CloneFixture(t *testing.T, src Fixture) Fixture {
	t.Helper()
	fx, err := cloneFixtureInto(src, t.TempDir(), filepath.Join(t.TempDir(), "origin.git"))
	if err != nil {
		t.Fatalf("CloneFixture: %v", err)
	}
	return fx
}

func CloneFixtureTo(src Fixture) (Fixture, error) {
	dir, err := os.MkdirTemp("", "gittest-clone-wt-")
	if err != nil {
		return Fixture{}, fmt.Errorf("mkdir clone worktree: %w", err)
	}
	originParent, err := os.MkdirTemp("", "gittest-clone-origin-")
	if err != nil {
		return Fixture{}, fmt.Errorf("mkdir clone origin parent: %w", err)
	}
	return cloneFixtureInto(src, dir, filepath.Join(originParent, "origin.git"))
}

func cloneFixtureInto(src Fixture, dir, origin string) (Fixture, error) {
	if src.Dir == "" {
		return Fixture{}, fmt.Errorf("empty source dir")
	}
	if err := copyTree(src.Dir, dir); err != nil {
		return Fixture{}, fmt.Errorf("copy fixture worktree: %w", err)
	}
	clonedOrigin := ""
	if src.Origin != "" {
		if err := os.MkdirAll(origin, 0o750); err != nil {
			return Fixture{}, fmt.Errorf("mkdir cloned origin: %w", err)
		}
		if err := copyTree(src.Origin, origin); err != nil {
			return Fixture{}, fmt.Errorf("copy fixture origin: %w", err)
		}
		clonedOrigin = origin
	}
	if err := rewriteClonedGitConfig(dir, src.Dir, src.Origin, dir, clonedOrigin); err != nil {
		return Fixture{}, fmt.Errorf("rewrite cloned git config: %w", err)
	}
	return Fixture{Dir: dir, Origin: clonedOrigin}, nil
}

func NewFixture() (Fixture, error) {
	if err := IsolateGit(); err != nil {
		return Fixture{}, err
	}
	if err := ensureSeeds(); err != nil {
		return Fixture{}, err
	}
	dir, err := os.MkdirTemp("", "gittest-wt-")
	if err != nil {
		return Fixture{}, fmt.Errorf("mkdir worktree: %w", err)
	}
	if err := copyTree(seeds.worktreeSeed, dir); err != nil {
		return Fixture{}, fmt.Errorf("copy worktree seed: %w", err)
	}
	originParent, err := os.MkdirTemp("", "gittest-origin-")
	if err != nil {
		return Fixture{}, fmt.Errorf("mkdir origin parent: %w", err)
	}
	origin := filepath.Join(originParent, "origin.git")
	if err := os.MkdirAll(origin, 0o750); err != nil {
		return Fixture{}, fmt.Errorf("mkdir origin: %w", err)
	}
	if err := copyTree(seeds.originSeed, origin); err != nil {
		return Fixture{}, fmt.Errorf("copy origin seed: %w", err)
	}
	if err := appendOriginRemote(dir, origin); err != nil {
		return Fixture{}, fmt.Errorf("attach origin: %w", err)
	}
	return Fixture{Dir: dir, Origin: origin}, nil
}

func Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := gitCombined(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func SetLocalConfig(dir, key, value string) error {
	_, err := gitCombined(dir, "config", key, value)
	return err
}

func gitCombined(dir string, args ...string) (string, error) {
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func ensureSeeds() error {
	seeds.once.Do(func() {
		seeds.err = buildSeeds()
	})
	return seeds.err
}

func buildSeeds() error {
	if err := IsolateGit(); err != nil {
		return err
	}
	worktree, err := os.MkdirTemp("", "gittest-seed-wt-")
	if err != nil {
		return fmt.Errorf("mkdir worktree seed: %w", err)
	}
	if err := adapters.GitInitMain(worktree); err != nil {
		return fmt.Errorf("GitInitMain: %w", err)
	}
	if err := configureRepoDir(worktree); err != nil {
		return fmt.Errorf("configure worktree seed: %w", err)
	}
	origin, err := os.MkdirTemp("", "gittest-seed-origin-")
	if err != nil {
		return fmt.Errorf("mkdir origin seed: %w", err)
	}
	if err := adapters.GitInitBareMain(origin); err != nil {
		return fmt.Errorf("GitInitBareMain: %w", err)
	}
	if err := configureRepoDir(origin); err != nil {
		return fmt.Errorf("configure origin seed: %w", err)
	}
	seeds.worktreeSeed = worktree
	seeds.originSeed = origin
	return nil
}

func configureRepoDir(dir string) error {
	for _, args := range [][]string{
		{"config", "user.email", isolatedAuthorEmail},
		{"config", "user.name", isolatedAuthorName},
		{"config", "commit.gpgsign", "false"},
		{"config", "tag.gpgsign", "false"},
		{"config", "gc.auto", "0"},
		{"config", "gc.autoDetach", "false"},
		{"config", "maintenance.auto", "false"},
		{"config", "receive.autogc", "false"},
		{"config", "credential.helper", ""},
	} {
		if _, err := gitCombined(dir, args...); err != nil {
			return fmt.Errorf("git %v: %w", args, err)
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return fs.WalkDir(os.DirFS(src), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(path))) //nolint:gosec // G304: src is a test seed directory
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600) //nolint:gosec // G703: copying test seed files
	})
}

func appendOriginRemote(repo, origin string) (err error) {
	cfg := filepath.Join(repo, ".git", "config")
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G302: test git config
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = fmt.Fprintf(f, "\n[remote \"origin\"]\n\turl = %s\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n", origin)
	return err
}

func rewriteClonedGitConfig(repo, oldDir, oldOrigin, newDir, newOrigin string) error {
	cfg := filepath.Join(repo, ".git", "config")
	data, err := os.ReadFile(cfg) //nolint:gosec // G304: path is the cloned test repo git config
	if err != nil {
		return err
	}
	s := string(data)
	if oldOrigin != "" {
		s = strings.ReplaceAll(s, oldOrigin, newOrigin)
	}
	if oldDir != "" {
		s = strings.ReplaceAll(s, oldDir, newDir)
	}
	return os.WriteFile(cfg, []byte(s), 0o600) //nolint:gosec // G703: cloned test repo git config
}
