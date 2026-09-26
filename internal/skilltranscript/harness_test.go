package skilltranscript

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type TestRepo struct {
	path   string
	armBin string
}

func NewTestRepo(t *testing.T) *TestRepo {
	t.Helper()

	tmpDir := t.TempDir()

	runCmd(tmpDir, "init", "-b", "main")

	runCmd(tmpDir, "config", "user.email", "test@example.com")
	runCmd(tmpDir, "config", "user.name", "Test User")
	runCmd(tmpDir, "config", "commit.gpgsign", "false")

	originParent := t.TempDir()
	origin := filepath.Join(originParent, "origin.git")
	runCmd(originParent, "init", "--bare", origin)
	runCmd(tmpDir, "remote", "add", "origin", origin)

	armBin := getArmBinary(t)
	if err := runCmdSafely(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "worker-init", "--check"); err != nil {
		runCmdWithEnv(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "worker-init")
	}

	readmeFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test Repository\n"), 0600); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}
	runCmd(tmpDir, "add", "-A")
	runCmd(tmpDir, "commit", "-m", "initial: setup test repo")

	runCmdWithEnv(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "bootstrap")

	runCmd(tmpDir, "add", "-A")
	runCmd(tmpDir, "commit", "-m", "initial: bootstrap armature")

	return &TestRepo{
		path:   tmpDir,
		armBin: armBin,
	}
}

func (tr *TestRepo) Path() string {
	return tr.path
}

func (tr *TestRepo) CreateStory(t *testing.T, title string) string {
	t.Helper()

	output := tr.runArm(t, "create",
		"--type", "story",
		"--title", title,
		"--dod", "All tasks done and reviewed",
		"--format", "json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse create story output as JSON: %v\nOutput: %q", err, output)
	}

	issueID, ok := result["id"].(string)
	if !ok {
		t.Fatalf("failed to extract story ID from JSON: %v", result)
	}

	tr.runArm(t, "materialize")

	return issueID
}

func (tr *TestRepo) HarnessCreateVerifiedTask(t *testing.T, parent, title string, scope []string) string {
	t.Helper()

	args := []string{
		"create",
		"--type", "task",
		"--title", title,
		"--parent", parent,
		"--dod", "Code reviewed and merged",
		"--acceptance", `[{"type":"test_passes"}]`,
		"--format", "json",
	}

	for _, s := range scope {
		args = append(args, "--scope", s)
	}

	output := tr.runArm(t, args...)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse create task output as JSON: %v\nOutput: %q", err, output)
	}

	issueID, ok := result["id"].(string)
	if !ok {
		t.Fatalf("failed to extract task ID from JSON: %v", result)
	}

	tr.runArm(t, "materialize")
	tr.runArm(t, "dag", "transition", "--issue", issueID, "--to", "verified")

	return issueID
}

func (tr *TestRepo) Ready(t *testing.T) []interface{} {
	t.Helper()

	output := tr.runArm(t, "ready", "--format", "json")

	var envelope struct {
		Issues []interface{} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("failed to parse ready output as JSON: %v", err)
	}
	if envelope.Issues == nil {
		t.Fatalf("ready envelope missing issues array: %s", output)
	}

	return envelope.Issues
}

func (tr *TestRepo) Claim(t *testing.T, issueID string, ttlMinutes int) string {
	t.Helper()

	worktreePath := filepath.Join(tr.path, ".worktrees", issueID)

	tr.runArm(t,
		"claim", issueID,
		"--ttl", fmt.Sprintf("%d", ttlMinutes),
		"--worktree")

	return worktreePath
}

func (tr *TestRepo) RenderContext(t *testing.T, issueID string) map[string]interface{} {
	t.Helper()

	output := tr.runArm(t, "render-context", issueID, "--format", "agent")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse render-context output as JSON: %v\nOutput: %q", err, output)
	}

	return result
}

func (tr *TestRepo) HarnessDriveTransition(t *testing.T, issueID, status, outcome string) {
	t.Helper()

	args := []string{"transition", issueID, "--to", status, "--outcome", outcome}
	if status == "done" {
		args = append(args, "--skip-delivery-gate")
	}
	tr.runArm(t, args...)
}

func (tr *TestRepo) ReviewPrepare(t *testing.T, issueID, baseSha, headSha, outputDir string) string {
	t.Helper()

	if outputDir == "" {
		outputDir = t.TempDir()
	}
	bundleFile := filepath.Join(outputDir, "bundle.json")

	tr.runArm(t,
		"review", "prepare",
		"--issue", issueID,
		"--base", baseSha,
		"--head", headSha,
		"--output", bundleFile)

	// #nosec G304 -- test reads a file path it just wrote under t.TempDir()
	content, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatalf("failed to read bundle file: %v", err)
	}

	var bundle map[string]interface{}
	if err := json.Unmarshal(content, &bundle); err != nil {
		t.Fatalf("failed to parse bundle as JSON: %v", err)
	}

	return bundleFile
}

func (tr *TestRepo) ReviewRecord(t *testing.T, issueID, assessmentFile, bundleFile string) {
	t.Helper()

	tr.runArm(t,
		"review", "record",
		"--issue", issueID,
		"--assessment", assessmentFile,
		"--bundle", bundleFile)
}

func (tr *TestRepo) runArm(t *testing.T, args ...string) string {
	t.Helper()

	return runCmdWithEnv(tr.path, map[string]string{"ARM_LOG_SLOT": "1"}, tr.armBin, args...)
}

func getArmBinary(t *testing.T) string {
	t.Helper()

	if armBin := os.Getenv("ARM_BIN"); armBin != "" {
		return armBin
	}

	armBin, err := exec.LookPath("arm")
	if err == nil {
		return armBin
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}

	binPath := filepath.Join(repoRoot, "bin", "arm")
	// #nosec G204 -- test helper builds the local binary with a fixed command line
	buildCmd := exec.CommandContext(context.Background(), "go", "build", "-ldflags", "-X main.Version=test", "-o", binPath, "./cmd/armature")
	buildCmd.Dir = repoRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build arm binary: %v\nOutput: %s", err, output)
	}

	return binPath
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find .git directory")
		}

		dir = parent
	}
}

func runCmd(dir string, args ...string) string {
	return runCmdWithEnv(dir, nil, "git", args...)
}

func runCmdWithEnv(dir string, env map[string]string, name string, args ...string) string {
	// #nosec G204 -- test helper invokes git/arm with test-controlled args
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("command failed: %s %v\nError: %v\nOutput: %s", name, args, err, string(output)))
	}

	return strings.TrimSpace(string(output))
}

func runCmdSafely(dir string, env map[string]string, name string, args ...string) error {
	// #nosec G204 -- test helper invokes git/arm with test-controlled args
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	_, err := cmd.CombinedOutput()
	return err
}
