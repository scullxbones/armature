package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testArmBin string

func TestMain(m *testing.M) {
	code := 1
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: getwd: %v\n", err)
		os.Exit(code)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "TestMain: go.mod not found")
			os.Exit(code)
		}
		dir = parent
	}
	wrapperDir, err := os.MkdirTemp("", "armature-test-arm-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: mkdir: %v\n", err)
		os.Exit(code)
	}
	testArmBin = filepath.Join(wrapperDir, "arm")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", testArmBin, "./cmd/armature")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: go build: %v\n%s\n", err, out)
		os.Exit(code)
	}
	if err := os.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: PATH: %v\n", err)
		os.Exit(code)
	}
	code = m.Run()
	if err := os.RemoveAll(wrapperDir); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: cleanup: %v\n", err)
	}
	os.Exit(code)
}

func TestBootstrapDeploySkillsDeploysFiles(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()

	err := deploySkills(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill")
}

func TestBootstrapDeploySkillsCreatesDestDir(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := filepath.Join(t.TempDir(), "nonexistent", "skills")

	err := deploySkills(src, dest)
	require.NoError(t, err)

	info, statErr := os.Stat(dest)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestBootstrapDeployFlatSkillsCreatesFlatMDFiles(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()

	require.NoError(t, deploySkills(src, dest))

	err := deployFlatSkills(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill", "flat md should contain SKILL.md body")
}

func TestBootstrapDeployFlatSkillsRewritesReferencePaths(t *testing.T) {
	src := fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo-skill\nSee `references/guide.md` for details.\n"),
		},
	}
	dest := t.TempDir()

	require.NoError(t, deploySkills(src, dest))

	err := deployFlatSkills(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill/references/guide.md", "flat md should have rewritten reference path")
	assert.NotContains(t, string(content), "`references/guide.md`", "flat md should not contain unrewritten reference path")
}

func TestBootstrapDeployPluginCreatesPluginJSON(t *testing.T) {
	src := makeBootstrapTestFSWithPlugin(t)
	dest := t.TempDir()

	err := deployPlugin(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "plugin.json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "armature")
}

func TestBootstrapCopyFileWorks(t *testing.T) {
	src := fstest.MapFS{
		"test.txt": {
			Data: []byte("test content"),
		},
	}
	dest := t.TempDir()
	destPath := filepath.Join(dest, "test.txt")

	err := copyFile(src, "test.txt", destPath)
	require.NoError(t, err)

	content, readErr := os.ReadFile(destPath)
	require.NoError(t, readErr)
	assert.Equal(t, "test content", string(content))
}

func TestBootstrapCopySkillWithRewrittenRefsWorks(t *testing.T) {
	src := fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo\nCheck references/guide.md\n"),
		},
	}
	dest := t.TempDir()
	destPath := filepath.Join(dest, "demo-skill.md")

	err := copySkillWithRewrittenRefs(src, "skills/demo-skill/SKILL.md", "demo-skill", destPath)
	require.NoError(t, err)

	content, readErr := os.ReadFile(destPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill/references/guide.md")
	assert.NotContains(t, string(content), "references/guide.md\" (shouldn't have unrewritten path)")
}

func makeBootstrapTestFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo-skill\nA demo skill.\n"),
		},
	}
}

func makeBootstrapTestFSWithPlugin(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"plugin.json": {
			Data: []byte(`{"name":"armature","description":"Test plugin"}`),
		},
	}
}

func TestBootstrapCommandRegistered(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--help"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "bootstrap")
}

func TestBootstrapCommandDefaultsToLocal(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(repo, ".armature"))
}

func TestRunRepoSetupCreatesStructure(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	armatureBase := filepath.Join(repo, ".armature")
	assert.DirExists(t, armatureBase)
	assert.DirExists(t, filepath.Join(armatureBase, "ops"))
	assert.DirExists(t, filepath.Join(armatureBase, "state"))
	assert.DirExists(t, filepath.Join(armatureBase, "state", "issues"))
	assert.DirExists(t, filepath.Join(armatureBase, "hooks"))
	assert.DirExists(t, filepath.Join(armatureBase, "templates"))
	assert.DirExists(t, filepath.Join(armatureBase, "review"))
}

func TestRunRepoSetupWritesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	gitignorePath := filepath.Join(repo, ".armature", ".gitignore")
	content, readErr := os.ReadFile(gitignorePath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "state/")
	assert.Contains(t, string(content), "gates/")
	assert.Contains(t, string(content), "review/")
	assert.Contains(t, string(content), fmt.Sprintf("# scaffolding-version: %d\n", ops.ScaffoldingVersion))
}

func TestBootstrapIgnoresGateAndReviewSidecars(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runRepoSetup(newRootCmd(), repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(opsWT, "gates"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(opsWT, "gates", "full-1.log"), []byte("ok\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(opsWT, "review"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(opsWT, "review", "TASK-1.json"), []byte("{}\n"), 0o600))

	status := runOutput(t, opsWT, "status", "--porcelain")
	assert.NotContains(t, status, "gates/")
	assert.NotContains(t, status, "review/")
}

func TestBootstrapCommitsOpsScaffoldingAndLeavesWorktreeClean(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	lsTree := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	assert.Contains(t, lsTree, "ops/SCHEMA")
	assert.Contains(t, lsTree, ".gitignore")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)

	schemaPath := filepath.Join(opsWT, "ops", "SCHEMA")
	require.NoError(t, os.WriteFile(schemaPath, []byte("# stale schema\n"), 0o600))
	_, err = runRepoSetup(cmd, repo)
	require.NoError(t, err)

	got, readErr := os.ReadFile(schemaPath)
	require.NoError(t, readErr)
	assert.Equal(t, ops.GenerateSchema(), string(got))
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
	assert.Contains(t, runOutput(t, repo, "log", "_armature", "--oneline"), "refresh ops scaffolding")
}

func TestBootstrapUntracksAlreadyCommittedSidecars(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	gateLog := filepath.Join(opsWT, "gates", "full-1.log")
	assessment := filepath.Join(opsWT, "review", "TASK-1.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(gateLog), 0o750))
	require.NoError(t, os.WriteFile(gateLog, []byte("ok\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(assessment), 0o750))
	require.NoError(t, os.WriteFile(assessment, []byte("{}\n"), 0o600))

	run(t, opsWT, "git", "add", "--force", "gates", "review")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: legacy committed sidecars")
	require.Contains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), "gates/full-1.log")

	_, err = runRepoSetup(cmd, repo)
	require.NoError(t, err)

	lsTree := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	assert.NotContains(t, lsTree, "gates/full-1.log")
	assert.NotContains(t, lsTree, "review/TASK-1.json")
	assert.FileExists(t, gateLog, "untracking must keep the local sidecar")
	assert.FileExists(t, assessment, "untracking must keep the local sidecar")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func TestBootstrapRefusesSidecarUntrackingWithUnrelatedStagedWork(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(opsWT, "gates"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(opsWT, "gates", "full-1.log"), []byte("ok\n"), 0o600))
	run(t, opsWT, "git", "add", "--force", "gates")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: legacy committed sidecar")

	workerLog := filepath.Join(opsWT, "ops", "worker-unrelated.log")
	require.NoError(t, os.WriteFile(workerLog, []byte("{}\n"), 0o600))
	run(t, opsWT, "git", "add", "ops/worker-unrelated.log")

	_, err = runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops/worker-unrelated.log")

	lsTree := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	assert.NotContains(t, lsTree, "ops/worker-unrelated.log", "unrelated staged work must not be swept into a cleanup commit")
	assert.Contains(t, lsTree, "gates/full-1.log", "the sidecar stays tracked until the index is clear")
}

func TestBootstrapKeepsHookTemplatesLocal(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	assert.NotContains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), ".sh.template")
	assert.FileExists(t, filepath.Join(opsWT, "hooks", "pre-commit.sh.template"), "templates must still be generated locally")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func TestBootstrapUntracksPreviouslyCommittedHookTemplates(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	templatePath := filepath.Join(opsWT, "hooks", "pre-commit.sh.template")
	run(t, opsWT, "git", "add", "--force", "hooks/pre-commit.sh.template")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: legacy committed hook template")
	require.Contains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), "hooks/pre-commit.sh.template")

	_, err = runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.NotContains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), "hooks/pre-commit.sh.template")
	assert.FileExists(t, templatePath, "untracking must keep the local template")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func TestBootstrapDoesNotDowngradeSchema(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	schemaPath := filepath.Join(opsWT, "ops", "SCHEMA")

	fromNewer := fmt.Sprintf("# Trellis Op Log Schema v1\n# scaffolding-version: %d\n# written by a newer arm\n", ops.ScaffoldingVersion+1)
	require.NoError(t, os.WriteFile(schemaPath, []byte(fromNewer), 0o600))
	run(t, opsWT, "git", "add", "ops/SCHEMA")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: schema from a newer arm")

	_, err = runRepoSetup(cmd, repo)
	require.NoError(t, err)

	got, readErr := os.ReadFile(schemaPath)
	require.NoError(t, readErr)
	assert.Equal(t, fromNewer, string(got), "an older generator must not overwrite a newer SCHEMA")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func TestBootstrapDoesNotDowngradeGitignore_REQ_OPSCLEAN_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	gitignorePath := filepath.Join(opsWT, ".gitignore")

	fromNewer := fmt.Sprintf("# scaffolding-version: %d\n# written by a newer arm\nstate/\n", ops.ScaffoldingVersion+1)
	require.NoError(t, os.WriteFile(gitignorePath, []byte(fromNewer), 0o600))
	run(t, opsWT, "git", "add", ".gitignore")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: gitignore from a newer arm")

	_, err = runRepoSetup(cmd, repo)
	require.NoError(t, err)

	got, readErr := os.ReadFile(gitignorePath)
	require.NoError(t, readErr)
	assert.Equal(t, fromNewer, string(got), "an older generator must not overwrite a newer ops .gitignore")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func interceptGitCommit(t *testing.T) func() {
	t.Helper()
	origPath := os.Getenv("PATH")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	wrapperDir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "commit" ]; then
    echo "injected git commit failure" >&2
    exit 1
  fi
done
exec %q "$@"
`, realGit)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+origPath)
	restore := func() {
		require.NoError(t, os.Setenv("PATH", origPath))
	}
	t.Cleanup(restore)
	return restore
}

func TestUntrackLocalOnlyPathsRestoresIndexAfterFailedCleanupCommit_REQ_OPSCLEAN_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	gateLog := filepath.Join(opsWT, "gates", "full-1.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(gateLog), 0o750))
	require.NoError(t, os.WriteFile(gateLog, []byte("ok\n"), 0o600))
	run(t, opsWT, "git", "add", "--force", "gates")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: legacy committed sidecar")

	client := adapters.New(opsWT)
	restoreGit := interceptGitCommit(t)
	err = untrackLocalOnlyPaths(client, "")
	restoreGit()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit local-only untracking")

	assert.True(t, client.IsTracked("gates/full-1.log") || client.IsTracked("gates"),
		"failed cleanup must restore the sidecar to the index")
	staged, stagedErr := client.StagedPaths()
	require.NoError(t, stagedErr)
	assert.Empty(t, staged, "failed cleanup must not leave staged deletions")
	assert.FileExists(t, gateLog)

	require.NoError(t, untrackLocalOnlyPaths(client, ""))
	assert.False(t, client.IsTracked("gates"))
	assert.False(t, client.IsTracked("gates/full-1.log"))
	assert.FileExists(t, gateLog)
	assert.NotContains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), "gates/full-1.log")
}

func TestUntrackLocalOnlyPathsFinishesStagedDeletionsOnRetry_REQ_OPSCLEAN_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	gateLog := filepath.Join(opsWT, "gates", "full-1.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(gateLog), 0o750))
	require.NoError(t, os.WriteFile(gateLog, []byte("ok\n"), 0o600))
	run(t, opsWT, "git", "add", "--force", "gates")
	run(t, opsWT, "git", "commit", "--no-verify", "-m", "chore: legacy committed sidecar")

	client := adapters.New(opsWT)
	require.NoError(t, client.RemoveFromIndex("gates"))
	require.False(t, client.IsTracked("gates"))
	staged, stagedErr := client.StagedPaths()
	require.NoError(t, stagedErr)
	require.NotEmpty(t, staged)

	require.NoError(t, untrackLocalOnlyPaths(client, ""))
	assert.False(t, client.IsTracked("gates"))
	assert.FileExists(t, gateLog)
	assert.NotContains(t, runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature"), "gates/full-1.log")
	assertOpsWorktreeHasNoTrackedDirt(t, opsWT)
}

func TestRunRepoSetupWritesSchemaFile(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	schemaPath := filepath.Join(repo, ".armature", "ops", "SCHEMA")
	_, statErr := os.Stat(schemaPath)
	require.NoError(t, statErr)
}

func TestRunRepoSetupInstallsHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	_, statErr := os.Stat(hookPath)
	require.NoError(t, statErr, "pre-commit hook should be installed")
}

func TestRunRepoSetupIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err1 := runRepoSetup(cmd, repo)
	require.NoError(t, err1)

	_, err2 := runRepoSetup(cmd, repo)
	require.NoError(t, err2, "second run should not fail")
}

func TestRunRepoSetupWritesConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	configPath := filepath.Join(repo, ".armature", "config.json")
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config.json should be created in worktree")
}

func TestBootstrapRemovesObsoletePrepareCommitMsgHook_REQ_HOOKMSG_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	hookPath := filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")
	require.NoError(t, os.MkdirAll(filepath.Dir(hookPath), 0o750))
	require.NoError(t, os.WriteFile(hookPath,
		[]byte("#!/bin/sh\n# armature:managed\nexit 0\n"), 0o755))

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	assert.NoFileExists(t, hookPath, "an Armature-managed prepare-commit-msg hook must be removed")
	assert.NoFileExists(t, filepath.Join(repo, ".armature", "hooks", "prepare-commit-msg.sh.template"),
		"the obsolete template must not be written back")
}

func TestBootstrapCommitsObsoleteHookTemplateDeletion_REQ_HOOKMSG_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	templateRel := "hooks/prepare-commit-msg.sh.template"
	templatePath := filepath.Join(opsWT, templateRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(templatePath), 0o750))
	require.NoError(t, os.WriteFile(templatePath, []byte("#!/bin/sh\n# armature:managed\nexit 0\n"), 0o600))
	opsGit := adapters.New(opsWT)
	run(t, opsWT, "git", "add", "--force", templateRel)
	require.NoError(t, opsGit.CommitPathsNoVerify("chore: plant legacy prepare-commit-msg template", templateRel))

	hookPath := filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\n# armature:managed\nexit 0\n"), 0o755))

	_, err = runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	assert.NoFileExists(t, templatePath, "tracked obsolete template must be removed")
	assert.NoFileExists(t, hookPath, "Armature-managed prepare-commit-msg hook must be removed")

	status := runOutput(t, opsWT, "status", "--porcelain")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if line == "" || strings.HasPrefix(line, "??") {
			continue
		}
		assert.Fail(t, "ops worktree has tracked dirty state after removing obsolete template: "+line)
	}

	lsTree := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	assert.NotContains(t, lsTree, templateRel, "obsolete template must not remain on _armature")
	assert.Contains(t, runOutput(t, repo, "log", "_armature", "--oneline"), "remove obsolete hook templates")
}

func TestBootstrapPreservesUserOwnedPrepareCommitMsgHook_REQ_HOOKMSG_1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	hookPath := filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")
	require.NoError(t, os.MkdirAll(filepath.Dir(hookPath), 0o750))
	userHook := "#!/bin/sh\n# my own hook\nexit 0\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(userHook), 0o755))

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, userHook, string(content), "a user-owned hook must be left untouched")
}

func TestInstallHooksExecutable(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	stat, statErr := os.Stat(hookPath)
	require.NoError(t, statErr)
	assert.NotZero(t, stat.Mode()&0o111, "hook should be executable")
}

func TestRunRepoSetupAlwaysCreatesDualBranchWorktree_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), "no .arm worktree should be created for a fresh init")

	assert.DirExists(t, filepath.Join(repo, ".armature"))

	configPath := filepath.Join(repo, ".armature", "config.json")
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config.json should exist in worktree")
}

func TestBootstrapDeployPluginUsesPluginName(t *testing.T) {
	src := makeBootstrapTestFSWithPlugin(t)

	pluginName, err := pluginNameFromFS(src)
	require.NoError(t, err)
	assert.Equal(t, "armature", pluginName, "plugin name should be extracted from plugin.json")
}

func TestBootstrapInvalidPlatformFailsBeforeRepoSetup(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codxe"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap with unknown platform should fail")

	armatureDir := filepath.Join(repo, ".armature")
	_, statErr := os.Stat(armatureDir)
	assert.True(t, os.IsNotExist(statErr), ".armature must not be created when platform validation fails")
}

func TestInstallHooksPreservesExistingUnmanagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userHookContent := "#!/bin/sh\n# User-managed pre-commit hook\necho 'User hook running'\n"
	userHookPath := filepath.Join(gitHooksDir, "pre-commit")
	require.NoError(t, os.WriteFile(userHookPath, []byte(userHookContent), 0o755))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	hookData, readErr := os.ReadFile(userHookPath)
	require.NoError(t, readErr)
	assert.Equal(t, userHookContent, string(hookData), "user-managed hook should not be overwritten")
}

func TestRunRepoSetupIdempotentDualBranchMode_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "config", "commit.gpgsign", "false")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))

	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))

	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	opsDir := filepath.Join(innerArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o750))
	testFile := filepath.Join(opsDir, "existing-issue.json")
	require.NoError(t, os.WriteFile(testFile, []byte(`{"id":"existing"}`), 0o600))
	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{config.StateDirName}))
	require.NoError(t, armGitClient.CommitWorktreeOp(config.StateDirName, "chore: simulate pre-existing dual-branch layout"))

	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), ".arm worktree should be migrated away")
	assert.DirExists(t, filepath.Join(repo, ".armature"), "collapsed .armature worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "ops directory should exist in the collapsed worktree")

	collapsedWorktreePath := filepath.Join(repo, ".armature")
	assertOpsWorktreeHasNoTrackedDirt(t, collapsedWorktreePath)

	lsTreeOut := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	for _, line := range strings.Split(strings.TrimSpace(lsTreeOut), "\n") {
		assert.False(t, strings.HasPrefix(line, ".armature/"), "branch tree must not contain nested .armature/ paths, got %q", line)
		assert.False(t, strings.HasPrefix(line, ".arm/"), "branch tree must not contain .arm/ paths, got %q", line)
	}
	assert.Contains(t, lsTreeOut, "ops/existing-issue.json", "the pre-migration ops data must be present at the root-level path")

	cloneDir := filepath.Join(t.TempDir(), "fresh-clone")
	runOutput(t, filepath.Dir(cloneDir), "clone", "--quiet", "--no-local", repo, cloneDir)
	runOutput(t, cloneDir, "checkout", "--quiet", "_armature")
	assert.FileExists(t, filepath.Join(cloneDir, "ops", "existing-issue.json"),
		"a fresh clone checked out to _armature must show ops data at the root-level path, not nested under .armature/")

	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	result2, err := runRepoSetup(cmd2, repo)
	require.NoError(t, err)
	assert.Equal(t, "already_initialized", result2.Status, "second run over the collapsed layout should be idempotent")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "ops directory should still exist after idempotent run")
}

func TestRunRepoSetupPrintsBackupSafetyGuidanceAfterDualBranchMigration(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "config", "commit.gpgsign", "false")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))

	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))

	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	opsDir := filepath.Join(innerArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "existing-issue.json"), []byte(`{"id":"existing"}`), 0o600))
	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{config.StateDirName}))
	require.NoError(t, armGitClient.CommitWorktreeOp(config.StateDirName, "chore: simulate pre-existing dual-branch layout"))

	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	var backupDirName string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".arm.collapsed-") {
			backupDirName = entry.Name()
			break
		}
	}
	require.NotEmpty(t, backupDirName, "expected a .arm.collapsed-<timestamp> backup directory to be left behind")

	output := buf.String()
	assert.Contains(t, output, backupDirName, "output should mention the backup directory path")
	assert.Contains(t, output, "safety snapshot", "output should explain the backup is a safety snapshot")
	assert.Contains(t, output, "_armature branch", "output should say the backup's contents are committed on _armature")
	assert.Contains(t, output, "safe to delete", "output should say the backup is safe to delete once verified")
}

func TestRunRepoSetupFreshInitDoesNotPrintBackupSafetyGuidance(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "safety snapshot", "fresh init performed no migration and must not print backup guidance")
}

func TestMigrateDualBranchToCollapsedRestoresUsableWorktreeAfterAddFailure_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))
	require.NoError(t, os.MkdirAll(filepath.Join(armWorktreePath, ".armature", "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(armWorktreePath, ".armature", "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{".armature"}))
	require.NoError(t, armGitClient.CommitWorktreeOp(".armature", "chore: create legacy layout"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	newWorktreePath := filepath.Join(repo, ".armature")
	wrapperDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
target=%q
sawTarget=0
for arg in "$@"; do
  if [ "$arg" = "$target" ]; then
    sawTarget=1
  fi
  if [ "$sawTarget" = "1" ] && [ "$arg" = "commit" ]; then
    exit 1
  fi
done
exec "$real_git" "$@"
`, realGit, newWorktreePath)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err = migrateDualBranchToCollapsed(repo)
	require.Error(t, err)

	gitDir := strings.TrimSpace(runOutput(t, armWorktreePath, "rev-parse", "--git-dir"))
	assert.Contains(t, gitDir, filepath.Join(".git", "worktrees"),
		"resolved git-dir must be .arm's own worktree entry, not a fallthrough to the outer repo: %s", gitDir)
	status := strings.TrimSpace(runOutput(t, armWorktreePath, "status", "--porcelain"))
	assert.Empty(t, status, "rollback must restore a clean legacy worktree")
}

func TestMigrateDualBranchToCollapsedPreservesCommitOnPostCommitConfigFailure(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))

	preexisting := map[string]string{
		"ops/root-op.json":             `{"id":"root"}`,
		"templates/root-template.md":   "root template\n",
		"config.json":                  `{"project_type":"go"}`,
		".armature/ops/legacy-op.json": `{"id":"legacy"}`,
		".armature/config.json":        `{"project_type":"legacy"}`,
	}
	for path, content := range preexisting {
		fullPath := filepath.Join(armWorktreePath, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
	}
	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{"ops", "templates", "config.json", ".armature"}))
	require.NoError(t, armGitClient.CommitWorktreeOp(".", "chore: create mixed legacy layout"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))
	preCollapseSHA, err := armGitClient.HeadSHA()
	require.NoError(t, err)

	wrapperDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
target=%q
sawTarget=0
for arg in "$@"; do
  if [ "$arg" = "$target" ]; then
    sawTarget=1
  fi
  if [ "$sawTarget" = "1" ] && [ "$arg" = "config" ]; then
    exit 1
  fi
done
exec "$real_git" "$@"
`, realGit, repo)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err = migrateDualBranchToCollapsed(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "committed successfully")

	newWorktreePath := filepath.Join(repo, config.StateDirName)
	newGitClient := adapters.New(newWorktreePath)
	postCommitSHA, err := newGitClient.HeadSHA()
	require.NoError(t, err)
	assert.NotEqual(t, preCollapseSHA, postCommitSHA, "collapse commit must remain on _armature, not be reset away")

	for _, path := range []string{"ops/root-op.json", "templates/root-template.md", "config.json"} {
		_, statErr := os.Stat(filepath.Join(newWorktreePath, path))
		assert.NoError(t, statErr, "expected %s to remain in collapsed worktree", path)
	}
}

func TestMigrateDualBranchToCollapsedIgnoresNestedRealRepo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	armPath := filepath.Join(repo, ".arm")
	require.NoError(t, os.MkdirAll(armPath, 0o750))
	run(t, armPath, "git", "init")
	run(t, armPath, "git", "config", "user.email", "test@test.com")
	run(t, armPath, "git", "config", "user.name", "Test")
	run(t, armPath, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.MkdirAll(filepath.Join(armPath, ".armature", "ops"), 0o750))
	run(t, armPath, "git", "commit", "--allow-empty", "-m", "unrelated nested repo")

	migrated, _, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err)
	assert.False(t, migrated, "a real nested repo at .arm/ must not be mistaken for a linked worktree")
	assert.DirExists(t, armPath, ".arm/ must be left in place, not renamed into a backup")
	assert.DirExists(t, filepath.Join(armPath, ".git"), ".arm/.git must remain a real repo directory")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command git %v failed: %s", args, out)
	return string(out)
}

func assertOpsWorktreeHasNoTrackedDirt(t *testing.T, opsWT string) {
	t.Helper()
	status := runOutput(t, opsWT, "status", "--porcelain")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if line == "" || strings.HasPrefix(line, "??") {
			continue
		}
		assert.Fail(t, "ops worktree has tracked dirty state: "+line)
	}
}

func TestRunRepoSetupDualBranchMigrationPreservesSources_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "config", "commit.gpgsign", "false")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))

	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))

	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	require.NoError(t, os.MkdirAll(filepath.Join(innerArmaturePath, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(innerArmaturePath, "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(innerArmaturePath, "sources"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(innerArmaturePath, "sources", "manifest.json"),
		[]byte(`{"uuid":"deadbeef","citations":[]}`), 0o600,
	))

	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{config.StateDirName}))
	require.NoError(t, armGitClient.CommitWorktreeOp(config.StateDirName, "chore: simulate pre-existing dual-branch layout with sources"))

	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	manifestPath := filepath.Join(repo, ".armature", "sources", "manifest.json")
	content, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "sources/manifest.json should be preserved at the collapsed worktree root")
	assert.Contains(t, string(content), "deadbeef")

	lsTreeOut := runOutput(t, repo, "ls-tree", "-r", "--name-only", "_armature")
	assert.Contains(t, lsTreeOut, "sources/manifest.json", "migrated sources/ data must be committed to _armature")
}

func TestRunRepoSetupConvergesEmptyArmWorktreeInOnePass_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "config", "commit.gpgsign", "false")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))

	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), "a single bootstrap pass must migrate away the empty .arm worktree")
	assert.DirExists(t, filepath.Join(repo, ".armature"), "collapsed .armature worktree should exist after one pass")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "ops directory should exist in the collapsed worktree")
	assert.FileExists(t, filepath.Join(repo, ".armature", ".gitignore"),
		"chained migration must (re-)write .gitignore into the final collapsed worktree so state/ stays ignored")
}

func TestInstallHooksReturnsSkippedHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userPreCommitHook := "#!/bin/sh\n# User-managed hook\necho 'User hook'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(gitHooksDir, "pre-commit"),
		[]byte(userPreCommitHook),
		0o755,
	))

	userPostCommitHook := "#!/bin/sh\n# Another user hook\necho 'Another hook'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(gitHooksDir, "post-commit"),
		[]byte(userPostCommitHook),
		0o755,
	))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")

	skipped, err := installHooks(repo, issuesDir)
	require.NoError(t, err)

	assert.NotEmpty(t, skipped, "skipped hooks list should not be empty")
	assert.Contains(t, skipped, "pre-commit", "should report pre-commit as skipped")
	assert.Contains(t, skipped, "post-commit", "should report post-commit as skipped")
}

func TestRunRepoSetupWarnsAboutSkippedHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userHookContent := "#!/bin/sh\n# User-managed pre-commit hook\necho 'User hook running'\n"
	userHookPath := filepath.Join(gitHooksDir, "pre-commit")
	require.NoError(t, os.WriteFile(userHookPath, []byte(userHookContent), 0o755))

	outBuf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "Warning:", "stderr should contain warning prefix")
	assert.Contains(t, errOutput, "pre-commit", "stderr should mention the skipped hook name")
	assert.Contains(t, errOutput, "not Armature-managed", "stderr should explain why it was skipped")
}

func TestBootstrapRespectsPersistentRepoFlag(t *testing.T) {
	repoPath := initTempRepo(t)
	run(t, repoPath, "git", "commit", "--allow-empty", "-m", "init")

	outBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)

	cmd.SetArgs([]string{"--repo", repoPath, "bootstrap"})

	err := cmd.Execute()
	require.NoError(t, err, "bootstrap with persistent --repo flag should succeed")

	assert.DirExists(t, filepath.Join(repoPath, ".armature"), ".armature should be initialized in the .arm worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "ops"), ".armature/ops should exist in worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "state"), ".armature/state should exist in worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "hooks"), ".armature/hooks should exist in worktree")
}

func TestBootstrapJSONSkippedHooksReported(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd1.Execute())

	hookPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\necho mine\n"), 0o755))

	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd2.Execute())

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf2.String()), &result))

	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok)
	skipped, ok := repoSetup["skipped_hooks"].([]interface{})
	require.True(t, ok, "skipped_hooks should be present when hooks are skipped")
	assert.Len(t, skipped, 1, "one hook should be skipped")
	assert.Equal(t, "post-commit", skipped[0])
}

func TestBootstrapJSONOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "bootstrap output should be valid JSON")

	assert.Contains(t, result, "repo_setup", "result should have repo_setup field")
	assert.Contains(t, result, "harness_setup", "result should have harness_setup field")

	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	assert.Contains(t, repoSetup, "status", "repo_setup should have status field")

	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")
	_ = harnessSetup
}

func TestBootstrapJSONRepoSetupStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd1.Execute()
	require.NoError(t, err)

	var result1 map[string]interface{}
	err = json.Unmarshal([]byte(buf1.String()), &result1)
	require.NoError(t, err)

	repoSetup1, ok := result1["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	status1, ok := repoSetup1["status"].(string)
	require.True(t, ok, "status should be a string")
	assert.Equal(t, "initialized", status1, "first bootstrap should report 'initialized'")

	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err = cmd2.Execute()
	require.NoError(t, err)

	var result2 map[string]interface{}
	err = json.Unmarshal([]byte(buf2.String()), &result2)
	require.NoError(t, err)

	repoSetup2, ok := result2["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	status2, ok := repoSetup2["status"].(string)
	require.True(t, ok, "status should be a string")
	assert.Equal(t, "already_initialized", status2, "second bootstrap should report 'already_initialized'")
}

func TestExecuteHarnessSetupSkipsUnownedConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	codexDir := filepath.Join(repo, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o755))
	codexPath := filepath.Join(codexDir, "config.toml")
	require.NoError(t, os.WriteFile(codexPath, []byte("# Some other config\nkey = \"value\"\n"), 0o600))

	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformCodex},
		Target:    "local",
		WithHooks: true,
	}
	plan, err := bootstrap.BuildPlan(req)
	require.NoError(t, err)

	results, err := executeHarnessSetup(cmd, plan, repo, false)
	require.NoError(t, err)

	var foundSkipped bool
	for _, result := range results {
		if result.Artifact == "harness_hook_config" && result.Status == "skipped" {
			foundSkipped = true
			assert.Equal(t, "codex", result.Platform)
			assert.Equal(t, "existing config not managed by Armature", result.Note)
			break
		}
	}
	assert.True(t, foundSkipped, "expected to find a skipped harness_hook_config result")
}

func TestInstallHooksSkipsUnmanagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	existingContent := `#!/bin/sh
# Some other pre-commit hook that's not managed by armature
echo "Running external pre-commit hook"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(existingContent), 0o755))

	issuesDir := filepath.Join(repo, ".armature")
	var skipped []string
	skipped, err = installHooks(repo, issuesDir)
	_ = skipped
	require.NoError(t, err)

	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(content), "hook should not have been overwritten")
}

func TestInstallHooksOverwritesManagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	oldContent := `#!/bin/sh
# armature:managed
# Old version of armature hook
echo "old"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(oldContent), 0o755))

	issuesDir := filepath.Join(repo, ".armature")
	var skipped2 []string
	skipped2, err = installHooks(repo, issuesDir)
	_ = skipped2
	require.NoError(t, err)

	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# armature:managed", "hook should have been overwritten")
	assert.NotContains(t, string(content), "echo \"old\"", "old content should be gone")
}

func TestBootstrapRejectsUnsupportedPlatformWithoutHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codex"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap --platform codex without --with-hooks should fail")
	assert.Contains(t, err.Error(), "no supported requested artifacts", "error should mention unsupported artifacts")

	armatureDir := filepath.Join(repo, ".armature")
	_, statErr := os.Stat(armatureDir)
	assert.True(t, os.IsNotExist(statErr), ".armature must not be created when platform validation fails")
}

func TestBootstrapNonTTYDefaultsToJSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "bootstrap output should be valid JSON when stdout is not a terminal")

	assert.Contains(t, result, "repo_setup", "result should have repo_setup field")
	assert.Contains(t, result, "harness_setup", "result should have harness_setup field")

	assert.NotContains(t, output, "Bootstrap complete.", "should not emit human text in non-TTY")
}

func TestBootstrapEmitsJSONOnRepoSetupError(t *testing.T) {
	nonexistentPath := filepath.Join(t.TempDir(), "no-git-here", ".armature")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", filepath.Dir(filepath.Dir(nonexistentPath)), "--format", "json"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap should fail when repo setup fails")

	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should be valid JSON even on repo setup failure")

	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")

	status, ok := repoSetup["status"].(string)
	require.True(t, ok, "repo_setup.status should be a string")
	assert.Equal(t, "error", status, "repo_setup.status should be 'error' when repo setup fails")

	errMsg, ok := repoSetup["error"].(string)
	require.True(t, ok, "repo_setup.error should be a string")
	assert.NotEmpty(t, errMsg, "repo_setup.error should contain the error message")

	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")
	assert.Empty(t, harnessSetup, "harness_setup should be empty when repo setup fails")
}

func TestBootstrapEmitsPartialJSONOnHarnessSetupError(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd1.Execute(), "initial bootstrap should succeed")

	claudePath := filepath.Join(repo, ".claude")
	require.NoError(t, os.RemoveAll(claudePath), "remove .claude dir")
	require.NoError(t, os.WriteFile(claudePath, []byte("blocking file"), 0o600), "create file at .claude path")
	t.Cleanup(func() {
		swallowErr(os.RemoveAll(claudePath))
	})

	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd2.Execute()
	require.Error(t, err, "bootstrap should fail due to directory creation error")

	output := buf2.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should be valid JSON even on partial failure")

	assert.Contains(t, result, "repo_setup", "partial JSON should have repo_setup field")
	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")

	_ = harnessSetup
}

func TestBootstrapReportsUnsupportedArtifactsInHumanFormat(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codex", "--with-hooks", "--format", "human"})

	err := cmd.Execute()
	require.NoError(t, err, "bootstrap should succeed")

	output := buf.String()

	assert.Contains(t, output, "unsupported", "output should mention unsupported artifacts")
	assert.Contains(t, output, "Bootstrap complete.", "output should end with completion message")
}

func TestBootstrapPersistentFormatFlagSetOnNonTTY(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "antigravity"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap should fail for unsupported platform")

	// Root SilenceErrors (ADR 0020) suppresses cobra's Error: line; the
	// Command Failure envelope is written by handleRootError in main.
	assert.Empty(t, errBuf.String())
	assert.NotEmpty(t, err.Error())
}

func TestRunRepoSetupMigratesLegacySingleBranchLayout_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testOpsFile := filepath.Join(legacyOpsPath, "test.json")
	testContent := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), "no .arm worktree should remain; chains straight to collapsed")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "new ops should be in the collapsed worktree")

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)

	var foundBackup bool
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			backupOpsFile := filepath.Join(repo, entry.Name(), "ops", "test.json")
			content, readErr := os.ReadFile(backupOpsFile)
			require.NoError(t, readErr, "original ops file should be in backup")
			assert.Equal(t, testContent, content, "backup should preserve original data")
			break
		}
	}
	assert.True(t, foundBackup, "should have .armature.migrated-<timestamp> backup directory")

	gitMarker, statErr := os.Stat(filepath.Join(legacyArmaturePath, ".git"))
	require.NoError(t, statErr, ".armature should now be the collapsed ops worktree")
	assert.False(t, gitMarker.IsDir(), ".armature/.git should be a worktree-pointer file")
}

func TestRunRepoSetupMigrationIsIdempotent_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testOpsFile := filepath.Join(legacyOpsPath, "test.json")
	testContent := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)

	assert.Contains(t, buf1.String(), "Migrated legacy single-branch")

	entries1, err := os.ReadDir(repo)
	require.NoError(t, err)
	var backupDir string
	for _, entry := range entries1 {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			backupDir = entry.Name()
			break
		}
	}
	require.NotEmpty(t, backupDir, "backup directory should exist after first run")

	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err)

	assert.NotContains(t, buf2.String(), "Migrated legacy single-branch", "second run should not attempt migration")

	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "ops should still be in the collapsed worktree")

	entries2, err := os.ReadDir(repo)
	require.NoError(t, err)
	var foundBackup bool
	for _, entry := range entries2 {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			assert.Equal(t, backupDir, entry.Name(), "should not create a new backup on second run")
			break
		}
	}
	assert.True(t, foundBackup, "backup directory should still exist after second run")

	backupOpsFile := filepath.Join(repo, backupDir, "ops", "test.json")
	content, err := os.ReadFile(backupOpsFile)
	require.NoError(t, err, "backup data should be preserved")
	assert.Equal(t, testContent, content, "backup should still contain original data")
}

func TestRunRepoSetupMigrationCopiesLegacyOpsData_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testFile1 := filepath.Join(legacyOpsPath, "issue001.json")
	testContent1 := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile1, testContent1, 0o600))

	testFile2 := filepath.Join(legacyOpsPath, "issue002.json")
	testContent2 := []byte(`{"id": "002", "title": "Legacy issue 2"}`)
	require.NoError(t, os.WriteFile(testFile2, testContent2, 0o600))

	legacyLogsDir := filepath.Join(legacyOpsPath, "logs")
	require.NoError(t, os.MkdirAll(legacyLogsDir, 0o750))
	testLog := filepath.Join(legacyLogsDir, "claim.log")
	logContent := []byte("claim: worker1")
	require.NoError(t, os.WriteFile(testLog, logContent, 0o600))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), ".arm worktree should not exist after chaining to collapsed")
	assert.DirExists(t, filepath.Join(repo, ".armature"), ".armature collapsed worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "new ops should be in collapsed worktree")

	newWorktreeOpsPath := filepath.Join(repo, ".armature", "ops")

	newFile1 := filepath.Join(newWorktreeOpsPath, "issue001.json")
	content1, err := os.ReadFile(newFile1)
	require.NoError(t, err, "legacy ops file issue001.json should be copied to new worktree")
	assert.Equal(t, testContent1, content1, "copied file should have same content as original")

	newFile2 := filepath.Join(newWorktreeOpsPath, "issue002.json")
	content2, err := os.ReadFile(newFile2)
	require.NoError(t, err, "legacy ops file issue002.json should be copied to new worktree")
	assert.Equal(t, testContent2, content2, "copied file should have same content as original")

	newLogsDir := filepath.Join(newWorktreeOpsPath, "logs")
	newLogFile := filepath.Join(newLogsDir, "claim.log")
	newLogContent, err := os.ReadFile(newLogFile)
	require.NoError(t, err, "legacy ops subdirectory should be copied to new worktree")
	assert.Equal(t, logContent, newLogContent, "copied subdirectory content should match original")

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)

	var foundBackup bool
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			backupOpsFile1 := filepath.Join(repo, entry.Name(), "ops", "issue001.json")
			backupContent1, readErr := os.ReadFile(backupOpsFile1)
			require.NoError(t, readErr, "original ops file should be in backup")
			assert.Equal(t, testContent1, backupContent1, "backup should preserve original data")
			break
		}
	}
	assert.True(t, foundBackup, "should have .armature.migrated-<timestamp> backup directory")
}

func TestRunRepoSetupMigrationCommitsLegacyOpsData_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testFile1 := filepath.Join(legacyOpsPath, "issue001.json")
	testContent1 := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile1, testContent1, 0o600))

	testFile2 := filepath.Join(legacyOpsPath, "issue002.json")
	testContent2 := []byte(`{"id": "002", "title": "Legacy issue 2"}`)
	require.NoError(t, os.WriteFile(testFile2, testContent2, 0o600))

	legacyLogsDir := filepath.Join(legacyOpsPath, "logs")
	require.NoError(t, os.MkdirAll(legacyLogsDir, 0o750))
	testLog := filepath.Join(legacyLogsDir, "claim.log")
	logContent := []byte("claim: worker1")
	require.NoError(t, os.WriteFile(testLog, logContent, 0o600))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), "no .arm worktree should remain; chains straight to collapsed")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "new ops should be in the collapsed worktree")

	gitLogCmd := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd.Dir = repo
	gitLogOut, err := gitLogCmd.Output()
	require.NoError(t, err, "should be able to read git log from _armature branch")

	logOutput := string(gitLogOut)
	assert.NotEmpty(t, logOutput, "_armature branch should have commits, not be empty")

	gitShowCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	showOutput := string(gitShowOut)
	assert.Contains(t, showOutput, "ops/issue001.json", "migrated ops file should be committed to _armature branch")
	assert.Contains(t, showOutput, "ops/issue002.json", "migrated ops file should be committed to _armature branch")
	assert.Contains(t, showOutput, "ops/logs/claim.log", "migrated ops subdirectory should be committed to _armature branch")

	gitShowFileCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:ops/issue001.json")
	gitShowFileCmd.Dir = repo
	gitShowFileOut, err := gitShowFileCmd.Output()
	require.NoError(t, err, "should be able to show committed ops file from _armature branch")
	assert.Equal(t, testContent1, gitShowFileOut, "committed ops file should have the correct content")
}

func TestRunRepoSetupMigrationIsIdempotent_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testFile := filepath.Join(legacyOpsPath, "issue001.json")
	testContent := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile, testContent, 0o600))

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)
	assert.Contains(t, buf1.String(), "Migrated legacy single-branch")

	gitLogCmd := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd.Dir = repo
	firstLogOut, err := gitLogCmd.Output()
	require.NoError(t, err)

	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err, "second bootstrap run over an already-migrated repo should not error")
	assert.NotContains(t, buf2.String(), "Migrated legacy single-branch",
		"second run should not re-migrate")

	gitLogCmd2 := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd2.Dir = repo
	secondLogOut, err := gitLogCmd2.Output()
	require.NoError(t, err)
	assert.Equal(t, string(firstLogOut), string(secondLogOut),
		"_armature branch history should be unchanged by a repeated bootstrap run")

	gitShowFileCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:ops/issue001.json")
	gitShowFileCmd.Dir = repo
	gitShowFileOut, err := gitShowFileCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, testContent, gitShowFileOut)
}

func TestRunRepoSetupMigratesLegacyConfig_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	legacyConfigPath := filepath.Join(legacyArmaturePath, "config.json")
	legacyConfig := config.Config{
		ProjectType:            "go",
		DefaultTTL:             120,
		TokenBudget:            3200,
		LowStakesPushThreshold: 10,
		Hooks:                  []config.HookConfig{},
	}
	require.NoError(t, config.WriteConfig(legacyConfigPath, legacyConfig))

	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	require.NoError(t, os.WriteFile(testOpsFile, []byte(`{"id":"001"}`), 0o600))

	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	assert.False(t, pathExists(filepath.Join(repo, ".arm")), "no .arm worktree should remain; chains straight to collapsed")
	assert.DirExists(t, filepath.Join(repo, ".armature"), ".armature worktree should exist")

	newConfigPath := filepath.Join(repo, ".armature", "config.json")
	migratedConfig, err := config.LoadConfig(newConfigPath)
	require.NoError(t, err, "config should be loadable from new location")

	assert.Equal(t, "go", migratedConfig.ProjectType, "ProjectType should be preserved")
	assert.Equal(t, 120, migratedConfig.DefaultTTL, "custom DefaultTTL should be preserved from legacy config (not reset to 60)")
	assert.Equal(t, 3200, migratedConfig.TokenBudget, "custom TokenBudget should be preserved from legacy config (not reset to 1600)")
	assert.Equal(t, 10, migratedConfig.LowStakesPushThreshold, "custom LowStakesPushThreshold should be preserved from legacy config (not reset to 5)")

	gitClient := adapters.New(repo)
	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after a successful legacy migration")
}

func TestRunRepoSetupMigration_DoesNotSweepUnrelatedStagedChanges(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	require.NoError(t, os.WriteFile(testOpsFile, []byte(`{"id":"001"}`), 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	unrelatedFile := filepath.Join(repo, "unrelated.txt")
	require.NoError(t, os.WriteFile(unrelatedFile, []byte("unrelated work in progress"), 0o600))
	run(t, repo, "git", "add", "unrelated.txt")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err, "bootstrap should refuse to run when the working tree has unrelated staged changes")
	assert.Contains(t, err.Error(), "dirty", "error should mention the dirty working tree")

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".armature.migrated-"),
			"no migration backup dir should be created when bootstrap refuses due to a dirty tree")
	}

	gitClient := adapters.New(repo)
	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.True(t, dirty, "unrelated staged change should remain uncommitted after the refused bootstrap")

	statusCmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	statusCmd.Dir = repo
	statusOut, statusErr := statusCmd.Output()
	require.NoError(t, statusErr)
	assert.Contains(t, string(statusOut), "unrelated.txt", "unrelated staged file should still be present in git status")
}

func TestDoctorCommandRunsOnLegacyRepo_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	testContent := []byte(`{"id": "issue-001", "title": "Legacy issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	gitCmd := exec.CommandContext(context.Background(), "git", "config", "armature.ops-worktree-path")
	gitCmd.Dir = repo
	err := gitCmd.Run()
	assert.Error(t, err, "legacy repo should NOT have armature.ops-worktree-path git config")

	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"--repo", repo, "doctor"})

	err = cmd.Execute()

	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "armature.ops-worktree-path must be set",
		"doctor should not fail with missing git config error on legacy repo")

	if err != nil {
		errMsg := err.Error()
		assert.NotContains(t, errMsg, "armature.ops-worktree-path must be set",
			"doctor error should not be about missing git config")
	}
}

func TestRunRepoSetupExcludesArmWorktreeFromGitTracking_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	content, readErr := os.ReadFile(excludePath)
	require.NoError(t, readErr, ".git/info/exclude should exist and be readable")

	excludeContent := string(content)
	assert.Contains(t, excludeContent, ".armature/", ".git/info/exclude should contain .armature/")

	firstCount := strings.Count(excludeContent, ".armature/")
	assert.Equal(t, 1, firstCount, "should have exactly one .armature/ entry")

	assert.Contains(t, excludeContent, ".worktrees/", ".git/info/exclude should contain .worktrees/")
	assert.Equal(t, 1, strings.Count(excludeContent, ".worktrees/"), "should have exactly one .worktrees/ entry")

	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err)

	content2, readErr2 := os.ReadFile(excludePath)
	require.NoError(t, readErr2)
	excludeContent2 := string(content2)
	assert.Equal(t, 1, strings.Count(excludeContent2, ".armature/"), "should still have exactly one .armature/ entry after second run (idempotent)")
	assert.Equal(t, 1, strings.Count(excludeContent2, ".worktrees/"), "should still have exactly one .worktrees/ entry after second run (idempotent)")
}

func TestCopyRecursiveDoesNotOverwriteExistingFiles_P2(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.txt")
	srcContent := []byte("source content")
	require.NoError(t, os.WriteFile(srcFile, srcContent, 0o600))

	dstFile := filepath.Join(dstDir, "test.txt")
	dstContent := []byte("destination content (should not be overwritten)")
	require.NoError(t, os.WriteFile(dstFile, dstContent, 0o600))

	skipped, err := copyRecursive(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped, "copyRecursive should report 1 skipped file")

	result, readErr := os.ReadFile(dstFile)
	require.NoError(t, readErr)
	assert.Equal(t, dstContent, result, "destination file should not be overwritten by copyRecursive")
}

func TestPushOpsRunEEmitsNoStderrOnFailure_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runRepoSetup(&cobra.Command{}, repo)
	require.NoError(t, err)

	out, errOutput, pushErr := runTrlsWithStderr(t, repo, "push-ops", "--format", "json")

	require.Error(t, pushErr, "push-ops should fail when no remote is configured")

	assert.Equal(t, "", errOutput, "push-ops RunE should write nothing to stderr; only main()'s top-level handler should render the error")
	assert.Equal(t, "", out, "stdout should be empty when push-ops fails")
}

func TestRunRepoSetupWarnsButSucceedsWhenExcludeFails_P2(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced, cannot simulate write failure")
	}

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	infoDir := filepath.Join(repo, ".git", "info")
	require.NoError(t, os.MkdirAll(infoDir, 0o750))
	excludePath := filepath.Join(infoDir, "exclude")
	require.NoError(t, os.WriteFile(excludePath, []byte("# existing\n"), 0o400))
	require.NoError(t, os.Chmod(excludePath, 0o400))
	t.Cleanup(func() {
		if err := os.Chmod(excludePath, 0o600); err != nil {
			t.Logf("cleanup: failed to restore exclude file permissions: %v", err)
		}
	})

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	result, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "bootstrap should succeed even when .git/info/exclude cannot be written")
	assert.NotEmpty(t, result.Status)

	assert.Contains(t, buf.String(), "Warning", "bootstrap should print a warning when the exclude write fails")
}

func TestCopyLegacyOpsToNewWorktreeMergesAppendOnlyLogs_P3(t *testing.T) {
	backupDir := t.TempDir()
	newIssuesDir := t.TempDir()
	newOpsDir := filepath.Join(newIssuesDir, "ops")
	require.NoError(t, os.MkdirAll(newOpsDir, 0o750))

	legacyOpsDir := filepath.Join(backupDir, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "merged.log"), []byte("a\nb\nc\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newOpsDir, "merged.log"), []byte("a\nx\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "existing.json"), []byte("legacy"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "new.json"), []byte("legacy"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newOpsDir, "existing.json"), []byte("already here"), 0o600))

	skippedCount, err := copyLegacyOpsToNewWorktree(backupDir, newIssuesDir)
	require.NoError(t, err)
	assert.Equal(t, 3, skippedCount, "one non-log collision plus two appended log lines should be counted")

	content, readErr := os.ReadFile(filepath.Join(newOpsDir, "existing.json"))
	require.NoError(t, readErr)
	assert.Equal(t, "already here", string(content))

	content, readErr = os.ReadFile(filepath.Join(newOpsDir, "new.json"))
	require.NoError(t, readErr)
	assert.Equal(t, "legacy", string(content))

	merged, readErr := os.ReadFile(filepath.Join(newOpsDir, "merged.log"))
	require.NoError(t, readErr)
	assert.Equal(t, "a\nx\nb\nc\n", string(merged))
}

func TestListMigrationBackupsSortsAndIgnoresUnreadableRepo_P3(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature.migrated-20260703010101"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature.migrated-20260703000101"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "unrelated"), 0o750))

	backups := listMigrationBackups(repo)
	require.Equal(t, []string{".armature.migrated-20260703000101", ".armature.migrated-20260703010101"}, backups)
	require.Nil(t, listMigrationBackups(filepath.Join(repo, "missing")))
}

func TestListMigrationBackupsIncludesCollapsedBackups_REQ_LNGHZN_S1(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".arm.collapsed-20260703010101"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature.migrated-20260703000101"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "unrelated"), 0o750))

	backups := listMigrationBackups(repo)
	require.Equal(t, []string{".arm.collapsed-20260703010101", ".armature.migrated-20260703000101"}, backups)
}

func TestRunRepoSetupNotesStrandedMigrationBackups_P3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature", "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".armature", "ops", "log.jsonl"), []byte(`{"op":"x"}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature.migrated-20260703010101"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".armature.migrated-20260703010101", "note.txt"), []byte("stale"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".armature.migrated-20260703000101"), 0o750))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Note: stranded migration backups remain: .armature.migrated-20260703000101, .armature.migrated-20260703010101")
}

func TestExcludeArmWorktreeFromGitExactLineMatch_P3(t *testing.T) {
	repo := t.TempDir()
	infoDir := filepath.Join(repo, ".git", "info")
	require.NoError(t, os.MkdirAll(infoDir, 0o750))
	excludePath := filepath.Join(infoDir, "exclude")

	require.NoError(t, os.WriteFile(excludePath, []byte("vendor.arm/\n"), 0o600))

	require.NoError(t, excludeArmWorktreeFromGit(repo))

	content, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Contains(t, lines, "vendor.arm/", "pre-existing unrelated line should be preserved")
	assert.Contains(t, lines, ".arm/", "the real .arm/ exclude entry should be appended despite the similar existing line")

	require.NoError(t, excludeArmWorktreeFromGit(repo))
	content2, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	count := 0
	for _, line := range strings.Split(string(content2), "\n") {
		if strings.TrimSpace(line) == ".arm/" {
			count++
		}
	}
	assert.Equal(t, 1, count, ".arm/ should not be duplicated on repeated calls")
}

func TestRunRepoSetupMigrationCommitsLegacyConfig_BUGFIX(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	legacyConfigPath := filepath.Join(legacyArmaturePath, "config.json")
	legacyConfig := config.Config{
		ProjectType:            "go",
		DefaultTTL:             120,
		TokenBudget:            3200,
		LowStakesPushThreshold: 10,
		Hooks:                  []config.HookConfig{},
	}
	require.NoError(t, config.WriteConfig(legacyConfigPath, legacyConfig))

	testOpsFile := filepath.Join(legacyOpsPath, "issue001.json")
	testOpsContent := []byte(`{"id": "001", "title": "Legacy issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testOpsContent, 0o600))

	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	gitShowCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	showOutput := string(gitShowOut)

	assert.Contains(t, showOutput, "config.json",
		"custom config.json MUST be committed to _armature branch so it's preserved for other clones")

	assert.Contains(t, showOutput, "ops/issue001.json",
		"migrated ops files should also be committed to _armature branch")

	gitShowConfigCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:config.json")
	gitShowConfigCmd.Dir = repo
	gitShowConfigOut, err := gitShowConfigCmd.Output()
	require.NoError(t, err, "should be able to show committed config from _armature branch")

	var committedConfig config.Config
	err = json.Unmarshal(gitShowConfigOut, &committedConfig)
	require.NoError(t, err, "committed config should be valid JSON")

	assert.Equal(t, "go", committedConfig.ProjectType, "ProjectType should be committed")
	assert.Equal(t, 120, committedConfig.DefaultTTL, "custom DefaultTTL should be committed (not default 60)")
	assert.Equal(t, 3200, committedConfig.TokenBudget, "custom TokenBudget should be committed (not default 1600)")
	assert.Equal(t, 10, committedConfig.LowStakesPushThreshold, "custom LowStakesPushThreshold should be committed (not default 5)")
}

func TestRunRepoSetupFreshBootstrap_CommitsConfigToArmatureBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	configPath := filepath.Join(repo, ".armature", "config.json")
	_, err = os.Stat(configPath)
	require.NoError(t, err, "config.json should exist in worktree")

	gitLsCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitLsCmd.Dir = repo
	gitLsOut, err := gitLsCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	lsOutput := string(gitLsOut)

	assert.Contains(t, lsOutput, "config.json",
		"config.json MUST be committed to _armature branch on fresh bootstrap")

	gitShowCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:config.json")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to show committed config from _armature branch")

	var committedConfig config.Config
	err = json.Unmarshal(gitShowOut, &committedConfig)
	require.NoError(t, err, "committed config should be valid JSON")

	assert.NotEmpty(t, committedConfig.ProjectType, "ProjectType should be set in default config")
}

func TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	testContent := []byte(`{"id": "001", "title": "Test issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	gitClient := adapters.New(repo)
	wasTrackedBefore := gitClient.IsTracked(".armature")
	require.True(t, wasTrackedBefore, ".armature should be tracked before migration")

	restoreGit := interceptGitCommit(t)
	migratedFlag, backupDir, preMigrationSHA, _, err := migrateLegacySingleBranchOps(repo)
	restoreGit()

	require.Error(t, err, "migration should fail because git commit was intercepted")
	assert.False(t, migratedFlag, "migrated flag should be false when migration fails")
	assert.Empty(t, backupDir, "backupDir should be empty when migration fails")
	assert.NotEmpty(t, preMigrationSHA, "preMigrationSHA should be recorded before rollback")

	assert.DirExists(t, legacyArmaturePath, ".armature directory should be restored after failed migration")

	restoredOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	restoredContent, err := os.ReadFile(restoredOpsFile)
	require.NoError(t, err, "original ops file should still exist after rollback")
	assert.Equal(t, testContent, restoredContent, "original ops file content should be unchanged")

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".armature.migrated-"),
			"backup directory should not exist after rollback (cleanup on error)")
	}

	isTrackedAfter := gitClient.IsTracked(".armature")
	assert.True(t, isTrackedAfter, ".armature should still be tracked after rollback (re-added to index on error)")

	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after rollback (no dangling staged removals)")

	migratedRetry, backupDirRetry, preMigrationSHARetry, _, errRetry := migrateLegacySingleBranchOps(repo)
	require.NoError(t, errRetry, "retry migration (without signing failure) should succeed")
	assert.True(t, migratedRetry, "retry migration should report success")
	assert.NotEmpty(t, backupDirRetry, "retry migration should return a backup dir path")
	assert.NotEmpty(t, preMigrationSHARetry, "retry migration should still record the pre-migration SHA")
	assert.DirExists(t, backupDirRetry, "retry backup directory should exist")
}

func TestRunRepoSetupMigratesTemplatesHooksReview_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "ops"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "templates"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "hooks"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "review"), 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "templates", "custom.md"), []byte("custom template"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "hooks", "custom-hook.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "review", "notes.md"), []byte("review notes"), 0o600))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	result, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Status)

	newIssuesDir := filepath.Join(repo, ".armature")

	templateContent, err := os.ReadFile(filepath.Join(newIssuesDir, "templates", "custom.md"))
	require.NoError(t, err, "legacy templates/ content should be migrated into the new worktree")
	assert.Equal(t, "custom template", string(templateContent))

	hookContent, err := os.ReadFile(filepath.Join(newIssuesDir, "hooks", "custom-hook.sh"))
	require.NoError(t, err, "legacy hooks/ content should be migrated into the new worktree")
	assert.Equal(t, "#!/bin/sh\necho hi\n", string(hookContent))

	reviewContent, err := os.ReadFile(filepath.Join(newIssuesDir, "review", "notes.md"))
	require.NoError(t, err, "legacy review/ content should be migrated into the new worktree")
	assert.Equal(t, "review notes", string(reviewContent))
}

func TestRunRepoSetupCommitsConfigWhenNotFreshInit_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".armature")
	issuesDir := worktreePath
	configPath := filepath.Join(issuesDir, "config.json")

	require.NoError(t, os.Remove(configPath))
	worktreeGitClient := adapters.New(worktreePath)
	require.NoError(t, worktreeGitClient.AddPaths([]string{"."}))
	require.NoError(t, worktreeGitClient.CommitPathsNoVerify("chore: simulate remote adoption without config.json", "."))

	opsFile := filepath.Join(issuesDir, "ops", "adopted-issue.json")
	require.NoError(t, os.WriteFile(opsFile, []byte(`{"id":"adopted"}`), 0o600))

	buf2 := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	result2, err := runRepoSetup(cmd2, repo)
	require.NoError(t, err)
	assert.Equal(t, "already_initialized", result2.Status, "ops/ is non-empty, so this run should not be a fresh init")

	collapsedWorktreePath := filepath.Join(repo, ".armature")
	collapsedConfigPath := filepath.Join(collapsedWorktreePath, "config.json")

	assert.FileExists(t, collapsedConfigPath, "config.json should be regenerated in the collapsed worktree")

	statusOut, err := exec.CommandContext(context.Background(), "git", "-C", collapsedWorktreePath, "status", "--porcelain", "config.json").Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(statusOut)), "regenerated config.json should be committed, not left as an uncommitted/untracked change")

	logOut, err := exec.CommandContext(context.Background(), "git", "-C", collapsedWorktreePath, "log", "-1", "--pretty=%s", "--", "config.json").Output()
	require.NoError(t, err)
	assert.Contains(t, string(logOut), "init armature config", "config.json should have a commit preserving it in _armature history")
}

func TestRunRepoSetupRollsBackMigrationWhenWorktreeAddFails_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testContent := []byte(`{"id": "001", "title": "Test issue"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "test-issue.json"), testContent, 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
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
`, realGit, filepath.Join(repo, ".armature"))
	require.NoError(t, os.WriteFile(wrapperPath, []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err = runRepoSetup(cmd, repo)
	require.Error(t, err, "runRepoSetup should fail because worktree add was intercepted")
	assert.Contains(t, err.Error(), "add .armature worktree")

	gitClient := adapters.New(repo)
	assert.DirExists(t, legacyArmaturePath, ".armature should be restored after rollback")
	restoredContent, readErr := os.ReadFile(filepath.Join(legacyOpsPath, "test-issue.json"))
	require.NoError(t, readErr, "legacy ops file should still exist after rollback")
	assert.Equal(t, testContent, restoredContent)
	assert.True(t, gitClient.IsTracked(".armature"), ".armature should be tracked again after rollback")

	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after rollback")
}

func TestRunRepoSetupRefusesOpsWorktree_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)

	armPath := filepath.Join(repo, ".armature")
	opsFile := filepath.Join(armPath, "ops", "worker-test.jsonl")
	opsContent := []byte(`{"op":"create"}`)
	require.NoError(t, os.WriteFile(opsFile, opsContent, 0o600))

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	_, err = runRepoSetup(cmd2, armPath)
	require.Error(t, err, "bootstrap targeting the ops worktree should be refused")
	assert.Contains(t, err.Error(), "_armature")

	content, readErr := os.ReadFile(opsFile)
	require.NoError(t, readErr, "ops data must not be renamed away")
	assert.Equal(t, opsContent, content)
	entries, err := os.ReadDir(armPath)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".armature.migrated-"),
			"no migration backup should be created inside the ops worktree")
	}
}

func TestRunRepoSetupMigrationFailureNamesBackupDir_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))

	legacyTemplates := filepath.Join(repo, ".armature", "templates")
	require.NoError(t, os.MkdirAll(legacyTemplates, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyTemplates, "t.md"), []byte("tmpl"), 0o600))
	require.NoError(t, os.Chmod(legacyTemplates, 0o000))
	t.Cleanup(func() {
		entries, readErr := os.ReadDir(repo)
		if readErr != nil {
			return
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".armature") {
				if chmodErr := os.Chmod(filepath.Join(repo, e.Name(), "templates"), 0o750); chmodErr != nil && !os.IsNotExist(chmodErr) {
					t.Logf("cleanup chmod: %v", chmodErr)
				}
			}
		}
	})

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"error after a post-rename failure must name the backup dir so data isn't silently stranded")
}

func TestRunRepoSetupMigrationCommitFailureNamesBackupDir_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
target=%q
cmd=""
found_target=""
skip=""
for arg in "$@"; do
  if [ -n "$skip" ]; then
    if [ "$skip" = "-C" ]; then
      found_target="$arg"
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
if [ "$cmd" = "commit" ] && [ "$found_target" = "$target" ]; then
  exit 1
fi
exec "$real_git" "$@"
`, realGit, filepath.Join(repo, ".armature"))
	require.NoError(t, os.WriteFile(wrapperPath, []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err = runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit migrated data")
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"commit failure after migration must name the backup dir so data isn't silently stranded")
}

func TestRunRepoSetupMigrationBackupNameCollision_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	opsContent := []byte(`{"op":"x"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), opsContent, 0o600))

	staleContent := []byte("stale")
	now := time.Now()
	var staleDirs []string
	for i := 0; i < 5; i++ {
		name := filepath.Join(repo, ".armature.migrated-"+now.Add(time.Duration(i)*time.Second).Format("20060102150405"))
		require.NoError(t, os.MkdirAll(name, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(name, "old.txt"), staleContent, 0o600))
		staleDirs = append(staleDirs, name)
	}

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "migration should succeed despite pre-existing backup dirs")

	for _, d := range staleDirs {
		content, readErr := os.ReadFile(filepath.Join(d, "old.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, staleContent, content, "pre-existing backup must not be clobbered")
	}

	migrated, readErr := os.ReadFile(filepath.Join(repo, ".armature", "ops", "log.jsonl"))
	require.NoError(t, readErr)
	assert.Equal(t, opsContent, migrated)
}

func TestRunRepoSetupCommittedRollbackNamesLeftoverBackup_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "tracked.jsonl"), []byte(`{"op":"a"}`), 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	untracked := []byte(`{"op":"untracked"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "untracked.jsonl"), untracked, 0o600))

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
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
`, realGit, filepath.Join(repo, ".armature"))
	require.NoError(t, os.WriteFile(wrapperPath, []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err = runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add .armature worktree")
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"rollback error must name the leftover backup holding untracked legacy files")

	entries, readErr := os.ReadDir(repo)
	require.NoError(t, readErr)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".armature.migrated-") {
			content, rerr := os.ReadFile(filepath.Join(repo, e.Name(), "ops", "untracked.jsonl"))
			require.NoError(t, rerr)
			assert.Equal(t, untracked, content)
			found = true
		}
	}
	assert.True(t, found, "backup dir should remain after committed rollback")
}

func TestRunRepoSetupWarnsOnUnreadableLegacyConfig_P3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".armature", "config.json"), []byte("{not json"), 0o600))

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "corrupt legacy config should not abort migration")

	combined := buf.String() + errBuf.String()
	assert.Contains(t, combined, "config", "user should be warned that legacy config was not migrated")
	assert.Contains(t, combined, "default", "warning should say defaults are used instead")
}

func TestBootstrap_ChainsLegacyToCollapsed_REQ_LNGHZN_S1_T3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	testOpsFile := filepath.Join(legacyOpsPath, "test.json")
	testContent := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	result, err := runRepoSetup(cmd, repo)

	require.NoError(t, err, "bootstrap should complete without error")

	gitMarkerPath := filepath.Join(repo, ".armature", ".git")
	gitMarkerInfo, statErr := os.Stat(gitMarkerPath)
	require.NoError(t, statErr, ".armature/.git should exist after collapsed migration")
	assert.False(t, gitMarkerInfo.IsDir(), ".armature/.git should be a worktree pointer file, not a directory")

	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), "collapsed layout should have .armature/ops/")
	assert.DirExists(t, filepath.Join(repo, ".armature", "state"), "collapsed layout should have .armature/state/")

	newOpsFile := filepath.Join(repo, ".armature", "ops", "test.json")
	newContent, readErr := os.ReadFile(newOpsFile)
	require.NoError(t, readErr, "original ops file should be preserved in collapsed layout")
	assert.Equal(t, testContent, newContent, "original ops file content should be unchanged")

	armWorktreePath := filepath.Join(repo, ".arm")
	_, armStatErr := os.Stat(armWorktreePath)
	assert.True(t, os.IsNotExist(armStatErr), ".arm/ worktree should not exist after chaining to collapsed layout")

	assert.Equal(t, "initialized", result.Status, "result should report fresh initialization")
}

func TestBootstrapHonorsConfiguredCustomCollapsedWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	customWorktreePath := filepath.Join(repo, ".ops")
	require.NoError(t, gitClient.AddWorktree("_armature", customWorktreePath))

	opsGitClient := adapters.New(customWorktreePath)
	require.NoError(t, os.MkdirAll(filepath.Join(customWorktreePath, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(customWorktreePath, "config.json"), []byte(`{"project_type":"go"}`), 0o600))
	require.NoError(t, opsGitClient.AddPaths([]string{"ops", "config.json"}))
	require.NoError(t, opsGitClient.CommitWorktreeOp(".", "chore: collapsed custom worktree"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", customWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "bootstrap should be idempotent against an existing custom collapsed worktree")

	configuredPath, err := adapters.GitConfig(repo, "armature.ops-worktree-path")
	require.NoError(t, err)
	assert.Equal(t, customWorktreePath, configuredPath, "bootstrap should keep using the configured custom worktree path")
	assert.NoDirExists(t, filepath.Join(repo, ".armature"), "bootstrap must not create a default .armature/ worktree alongside a valid custom one")
}

func TestRollbackLegacyMigrationRefusesResetWhenHeadOnArmature_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	preSHA := strings.TrimSpace(runOutput(t, repo, "rev-parse", "HEAD"))

	run(t, repo, "git", "commit", "--allow-empty", "-m", "chore: migrate legacy .armature to dual-branch layout")

	run(t, repo, "git", "checkout", "--orphan", "_armature")
	run(t, repo, "git", "commit", "--no-verify", "--allow-empty", "-m", "chore: init armature issues branch")
	armatureSHABefore := strings.TrimSpace(runOutput(t, repo, "rev-parse", "_armature"))

	backupDir := t.TempDir()
	err := rollbackLegacyMigration(repo, backupDir, preSHA, true)
	require.Error(t, err, "rollback must refuse to reset --hard while HEAD is on _armature")
	assert.Contains(t, err.Error(), "_armature")
	assert.Contains(t, err.Error(), backupDir, "error must name the backup dir for manual recovery")

	armatureSHAAfter := strings.TrimSpace(runOutput(t, repo, "rev-parse", "_armature"))
	assert.Equal(t, armatureSHABefore, armatureSHAAfter, "_armature ref must be untouched by the refused reset")
}

func TestRunRepoSetupRollsBackLegacyMigrationWhenCollapseMigrationFails_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testContent := []byte(`{"id": "001", "title": "Test issue"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "test-issue.json"), testContent, 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	armWorktreePath := filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))
	require.NoError(t, os.MkdirAll(filepath.Join(armWorktreePath, ".armature", "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(armWorktreePath, ".armature", "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{".armature"}))
	require.NoError(t, armGitClient.CommitWorktreeOp(".armature", "chore: create legacy dual-branch layout"))
	require.NoError(t, os.WriteFile(filepath.Join(armWorktreePath, ".armature", "ops", "issue.json"), []byte(`{"id":"1","modified":true}`), 0o600))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err, "runRepoSetup should fail because the dual-branch worktree is dirty")
	assert.Contains(t, err.Error(), "migrate dual-branch layout to collapsed")

	assert.DirExists(t, legacyArmaturePath, ".armature should be restored after rollback")
	restoredContent, readErr := os.ReadFile(filepath.Join(legacyOpsPath, "test-issue.json"))
	require.NoError(t, readErr, "legacy ops file should still exist after rollback")
	assert.Equal(t, testContent, restoredContent)
	assert.True(t, gitClient.IsTracked(".armature"), ".armature should be tracked again after rollback")

	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree at repo root should be clean after rollback")
}

func TestBootstrapCustomCollapsedWorktreeExcludedByOwnBasename_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	customWorktreePath := filepath.Join(repo, ".ops")
	require.NoError(t, gitClient.AddWorktree("_armature", customWorktreePath))

	opsGitClient := adapters.New(customWorktreePath)
	require.NoError(t, os.MkdirAll(filepath.Join(customWorktreePath, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(customWorktreePath, "config.json"), []byte(`{"project_type":"go"}`), 0o600))
	require.NoError(t, opsGitClient.AddPaths([]string{"ops", "config.json"}))
	require.NoError(t, opsGitClient.CommitWorktreeOp(".", "chore: collapsed custom worktree"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", customWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	excludeContent, readErr := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	require.NoError(t, readErr)
	assert.Contains(t, string(excludeContent), ".ops/", ".git/info/exclude should exclude the custom worktree by its own basename")
	assert.NotContains(t, string(excludeContent), ".armature/", ".git/info/exclude should not exclude the unused default .armature/ path")
}

func TestRunRepoSetupRefusesNestedUnmigratedCustomWorktree_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	customWorktreePath := filepath.Join(repo, ".ops")
	require.NoError(t, gitClient.AddWorktree("_armature", customWorktreePath))

	opsGitClient := adapters.New(customWorktreePath)
	nestedArmaturePath := filepath.Join(customWorktreePath, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(nestedArmaturePath, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(nestedArmaturePath, "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(nestedArmaturePath, "config.json"), []byte(`{"project_type":"go"}`), 0o600))
	require.NoError(t, opsGitClient.AddPaths([]string{".armature"}))
	require.NoError(t, opsGitClient.CommitWorktreeOp(".armature", "chore: nested pre-collapse layout"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", customWorktreePath))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err, "bootstrap should refuse a pre-collapse custom ops worktree")
	assert.Contains(t, err.Error(), "pre-collapse custom ops worktree")
	assert.Contains(t, err.Error(), "supports only .arm")
}

func TestRunRepoSetupCustomCollapsedWorktreeCorruptConfigDoesNotCorruptState_REQ_LNGHZN_S1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	customWorktreePath := filepath.Join(repo, ".ops")
	require.NoError(t, gitClient.AddWorktree("_armature", customWorktreePath))

	opsGitClient := adapters.New(customWorktreePath)
	require.NoError(t, os.MkdirAll(filepath.Join(customWorktreePath, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(customWorktreePath, "config.json"), []byte(`{not valid json`), 0o600))
	require.NoError(t, opsGitClient.AddPaths([]string{"ops", "config.json"}))
	require.NoError(t, opsGitClient.CommitWorktreeOp(".", "chore: collapsed custom worktree with bad config"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", customWorktreePath))

	preSHA := strings.TrimSpace(runOutput(t, customWorktreePath, "rev-parse", "HEAD"))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	swallowErr(err)

	gitDir := strings.TrimSpace(runOutput(t, customWorktreePath, "rev-parse", "--git-dir"))
	assert.Contains(t, gitDir, filepath.Join(".git", "worktrees"),
		"custom worktree must remain a genuinely registered worktree")
	postSHA := strings.TrimSpace(runOutput(t, customWorktreePath, "rev-parse", "HEAD"))
	assert.Equal(t, preSHA, postSHA, "custom worktree history must not be rewound")
}

func TestPostCommitTemplateDelegatesToHookRun_REQ_HKDLG_T1(t *testing.T) {
	require.Contains(t, postCommitHookTemplate, "# armature:managed")

	var body []string
	for _, line := range strings.Split(postCommitHookTemplate, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		body = append(body, trimmed)
	}
	require.Equal(t, []string{
		"unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR",
		"arm hook run post-commit",
	}, body)
	assert.NotContains(t, postCommitHookTemplate, "arm heartbeat")
	assert.NotContains(t, postCommitHookTemplate, "arm push-ops")
}

func TestPostCommitRecordsHeartbeatForActiveClaim_REQ_HKDLG_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "heartbeat probe", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "task-01")
	require.DirExists(t, wt)

	armBin := buildWorktreeArm(t)
	wrapperDir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
exec %q "$@"
`, armBin)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "arm"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	commitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	commit := exec.CommandContext(commitCtx, "git", "commit", "--allow-empty", "-m", "probe heartbeat")
	commit.Dir = wt
	var stdout, stderr bytes.Buffer
	commit.Stdout = &stdout
	commit.Stderr = &stderr
	err = commit.Run()
	require.NoError(t, err, "git commit stdout=%q stderr=%q", stdout.String(), stderr.String())
	assert.NotContains(t, stdout.String(), "GENERAL-1")

	ctx := getTestContext(t, repo)
	_, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	logged, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	heartbeats := 0
	for _, op := range logged {
		if op.Type == ops.OpHeartbeat && op.TargetID == "task-01" {
			heartbeats++
		}
	}
	assert.Equal(t, 1, heartbeats,
		"expected exactly one heartbeat for task-01 (no recursion) in %s; stdout=%q stderr=%q",
		logPath, stdout.String(), stderr.String())
}

func TestPreCommitTemplateDelegatesToHookRun_REQ_HKDLG_T3(t *testing.T) {
	require.Contains(t, preCommitHookTemplate, "# armature:managed")

	var body []string
	for _, line := range strings.Split(preCommitHookTemplate, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		body = append(body, trimmed)
	}
	require.Equal(t, []string{
		"unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR",
		"current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)",
		"if [ \"$current_branch\" = \"_armature\" ]; then",
		"exit 0",
		"fi",
		"command -v arm >/dev/null 2>&1 || exit 0",
		"arm hook run pre-commit",
	}, body)
	assert.NotContains(t, preCommitHookTemplate, "git diff --cached")
	assert.NotContains(t, preCommitHookTemplate, "grep -q")
}

func TestPreCommitRefusalReachesStderrWithRemediation_REQ_HKDLG_T3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	armBin := buildWorktreeArm(t)
	wrapperDir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
exec %q "$@"
`, armBin)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "arm"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	probeRel := filepath.Join("leaked", ".armature", "ops", "probe.log")
	writeFile(t, repo, probeRel, "ops must not land on a code branch\n")
	run(t, repo, "git", "add", "--force", probeRel)

	commitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	commit := exec.CommandContext(commitCtx, "git", "commit", "-m", "should be refused")
	commit.Dir = repo
	var stdout, stderr bytes.Buffer
	commit.Stdout = &stdout
	commit.Stderr = &stderr
	err = commit.Run()
	require.Error(t, err, "pre-commit must fail-loud; stdout=%q stderr=%q", stdout.String(), stderr.String())
	assert.NotContains(t, stdout.String(), "GENERAL-1")
	assert.Contains(t, stderr.String(), "ERROR: Refusing to commit .armature/ops/")
	assert.Contains(t, stderr.String(), "arm bootstrap --dual-branch")
}

func buildWorktreeArm(t *testing.T) string {
	t.Helper()
	if testArmBin != "" {
		return testArmBin
	}
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from test working directory")
		}
		dir = parent
	}
	dest := filepath.Join(t.TempDir(), "arm")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", dest, "./cmd/armature")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build: %s", out)
	return dest
}
