package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type migrationMatrixCase struct {
	name              string
	tracked           bool
	injectCommitFail  bool
	injectWorktreeBad bool
}

func TestMigrationInvariantMatrix_P1(t *testing.T) {
	cases := []migrationMatrixCase{
		{name: "tracked-success", tracked: true},
		{name: "tracked-commit-failure", tracked: true, injectCommitFail: true},
		{name: "untracked-success", tracked: false},
		{name: "untracked-worktree-add-failure", tracked: false, injectWorktreeBad: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initTempRepo(t)
			run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
			originalPath := os.Getenv("PATH")

			legacyArmaturePath := filepath.Join(repo, ".armature")
			legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
			require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

			legacyContent := []byte(`{"id":"matrix","op":"create"}`)
			legacyFile := filepath.Join(legacyOpsPath, "matrix.jsonl")
			require.NoError(t, os.WriteFile(legacyFile, legacyContent, 0o600))

			if tc.tracked {
				run(t, repo, "git", "add", ".armature")
				run(t, repo, "git", "commit", "-m", "legacy setup")
			}

			if tc.injectCommitFail {
				wrapperDir := installCommitFailureWrapper(t, repo)
				t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Cleanup(func() {
					require.NoError(t, os.Setenv("PATH", originalPath))
				})
			}

			if tc.injectWorktreeBad {
				wrapperDir := installWorktreeAddFailureWrapper(t, filepath.Join(repo, ".armature"))
				t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Cleanup(func() {
					require.NoError(t, os.Setenv("PATH", originalPath))
				})
			}

			buf := new(bytes.Buffer)
			cmd := newRootCmd()
			cmd.SetOut(buf)

			_, err := runRepoSetup(cmd, repo)
			if tc.injectCommitFail || tc.injectWorktreeBad {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assertLegacyContentFindable(t, repo, legacyContent, tc.tracked)

			if tc.injectCommitFail || tc.injectWorktreeBad {
				require.NoError(t, os.Setenv("PATH", originalPath))
				buf2 := new(bytes.Buffer)
				cmd2 := newRootCmd()
				cmd2.SetOut(buf2)
				_, err2 := runRepoSetup(cmd2, repo)
				require.NoError(t, err2)
			}

			assertLegacyContentFindable(t, repo, legacyContent, tc.tracked)
		})
	}
}

func installCommitFailureWrapper(t *testing.T, repo string) string {
	t.Helper()

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)

	script := `#!/bin/sh
real_git=%q
repo=%q
cmd=""
target=""
skip=""
for arg in "$@"; do
  if [ -n "$skip" ]; then
    if [ "$skip" = "-C" ]; then
      target="$arg"
    fi
    skip=""
    continue
  fi
  case "$arg" in
    -C|-c)
      skip="$arg"
      continue
      ;;
    commit)
      cmd="$arg"
      ;;
  esac
done
if [ "$cmd" = "commit" ] && [ "$target" = "$repo" ]; then
  exit 1
fi
exec "$real_git" "$@"
`
	content := strings.ReplaceAll(fmt.Sprintf(script, realGit, repo), "\r\n", "\n")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(content), 0o755))
	return wrapperDir
}

func installWorktreeAddFailureWrapper(t *testing.T, target string) string {
	t.Helper()

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)

	script := `#!/bin/sh
real_git=%q
target=%q
prev=""
for arg in "$@"; do
  if [ "$prev" = "add" ] && [ "$arg" = "$target" ]; then
    exit 1
  fi
  prev="$arg"
done
exec "$real_git" "$@"
`
	content := strings.ReplaceAll(fmt.Sprintf(script, realGit, target), "\r\n", "\n")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(content), 0o755))
	return wrapperDir
}

func assertLegacyContentFindable(t *testing.T, repo string, legacyContent []byte, tracked bool) {
	t.Helper()

	worktreeContent, err := os.ReadFile(filepath.Join(repo, ".armature", "ops", "matrix.jsonl"))
	if err == nil {
		assert.Equal(t, legacyContent, worktreeContent)
		return
	}

	restoredContent, restoredErr := os.ReadFile(filepath.Join(repo, ".armature", "ops", "matrix.jsonl"))
	if restoredErr == nil {
		assert.Equal(t, legacyContent, restoredContent)
		return
	}

	entries, readErr := os.ReadDir(repo)
	require.NoError(t, readErr)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			backupContent, backupErr := os.ReadFile(filepath.Join(repo, entry.Name(), "ops", "matrix.jsonl"))
			require.NoError(t, backupErr)
			assert.Equal(t, legacyContent, backupContent)
			return
		}
	}

	t.Fatalf("legacy content not findable for tracked=%v", tracked)
}
