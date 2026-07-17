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

// TestRepo represents a temporary git repository for testing armature commands.
type TestRepo struct {
	path   string
	armBin string
}

// NewTestRepo creates a new temporary git repository initialized for armature.
// It returns a TestRepo with the path to the repository and the path to the arm binary.
// The caller must call Close() to clean up resources.
func NewTestRepo(t *testing.T) *TestRepo {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()

	// Initialize as git repo
	runCmd(tmpDir, "init")

	// Configure git user and disable GPG signing
	runCmd(tmpDir, "config", "user.email", "test@example.com")
	runCmd(tmpDir, "config", "user.name", "Test User")
	runCmd(tmpDir, "config", "commit.gpgsign", "false")

	// Initialize worker
	armBin := getArmBinary(t)
	// Try worker-init --check first; if it fails, run full init
	if err := runCmdSafely(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "worker-init", "--check"); err != nil {
		// Check failed, run full init
		runCmdWithEnv(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "worker-init")
	}

	// Create initial commit before bootstrap
	// (bootstrap expects at least one commit to exist)
	readmeFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test Repository\n"), 0600); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}
	runCmd(tmpDir, "add", "-A")
	runCmd(tmpDir, "commit", "-m", "initial: setup test repo")
	// Git's default initial branch is configurable; normalize it explicitly so
	// worktree-based transcript tests are independent of the runner's config.
	runCmd(tmpDir, "branch", "-M", "main")

	// Bootstrap armature
	runCmdWithEnv(tmpDir, map[string]string{"ARM_LOG_SLOT": "1"}, armBin, "bootstrap")

	// Commit bootstrap files
	runCmd(tmpDir, "add", "-A")
	runCmd(tmpDir, "commit", "-m", "initial: bootstrap armature")

	return &TestRepo{
		path:   tmpDir,
		armBin: armBin,
	}
}

// Path returns the root path of the test repository.
func (tr *TestRepo) Path() string {
	return tr.path
}

// CreateStory creates a new story issue and returns its ID.
func (tr *TestRepo) CreateStory(t *testing.T, title string) string {
	t.Helper()

	output := tr.runArm(t, "create",
		"--type", "story",
		"--title", title,
		"--dod", "All tasks done and reviewed",
		"--format", "json")

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse create story output as JSON: %v\nOutput: %q", err, output)
	}

	// Extract issue ID
	issueID, ok := result["id"].(string)
	if !ok {
		t.Fatalf("failed to extract story ID from JSON: %v", result)
	}

	// Materialize state so the issue can be found by subsequent commands
	tr.runArm(t, "materialize")

	return issueID
}

// CreateTask creates a new task issue under a parent story and returns its ID.
func (tr *TestRepo) CreateTask(t *testing.T, parent, title string, scope []string) string {
	t.Helper()

	args := []string{
		"create",
		"--type", "task",
		"--title", title,
		"--parent", parent,
		"--dod", "Code reviewed and merged",
		"--format", "json",
	}

	for _, s := range scope {
		args = append(args, "--scope", s)
	}

	output := tr.runArm(t, args...)

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse create task output as JSON: %v\nOutput: %q", err, output)
	}

	// Extract issue ID
	issueID, ok := result["id"].(string)
	if !ok {
		t.Fatalf("failed to extract task ID from JSON: %v", result)
	}

	// Materialize state so the issue can be found by subsequent commands
	tr.runArm(t, "materialize")

	return issueID
}

// Ready returns the list of ready issues in JSON format.
func (tr *TestRepo) Ready(t *testing.T) []interface{} {
	t.Helper()

	output := tr.runArm(t, "ready", "--format", "json")

	var result []interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse ready output as JSON: %v", err)
	}

	return result
}

// Claim claims an issue with a worktree and returns the worktree path.
func (tr *TestRepo) Claim(t *testing.T, issueID string, ttlMinutes int) string {
	t.Helper()

	worktreePath := filepath.Join(tr.path, "worktrees", issueID)

	tr.runArm(t,
		"claim", issueID,
		"--ttl", fmt.Sprintf("%d", ttlMinutes),
		"--worktree", worktreePath)

	return worktreePath
}

// RenderContext renders the context for an issue in agent format and returns the parsed JSON.
func (tr *TestRepo) RenderContext(t *testing.T, issueID string) map[string]interface{} {
	t.Helper()

	output := tr.runArm(t, "render-context", issueID, "--format", "agent")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse render-context output as JSON: %v\nOutput: %q", err, output)
	}

	return result
}

// Transition transitions an issue to a new status.
func (tr *TestRepo) Transition(t *testing.T, issueID, status, outcome string) {
	t.Helper()

	tr.runArm(t,
		"transition", issueID,
		"--to", status,
		"--outcome", outcome)
}

// ReviewPrepare prepares a review bundle for an issue.
// If outputDir is empty, uses t.TempDir(); otherwise stores the bundle in outputDir.
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

	// Verify the file exists and contains valid JSON
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

// ReviewRecord records a conformance assessment for an issue.
func (tr *TestRepo) ReviewRecord(t *testing.T, issueID, assessmentFile, bundleFile string) {
	t.Helper()

	tr.runArm(t,
		"review", "record",
		"--issue", issueID,
		"--assessment", assessmentFile,
		"--bundle", bundleFile)
}

// runArm runs an arm command in the test repository and returns its output.
// It fails the test if the command exits with a non-zero status.
func (tr *TestRepo) runArm(t *testing.T, args ...string) string {
	t.Helper()

	return runCmdWithEnv(tr.path, map[string]string{"ARM_LOG_SLOT": "1"}, tr.armBin, args...)
}

// getArmBinary returns the path to the arm binary for testing.
// It first checks the ARM_BIN environment variable (set by the Makefile during tests).
// If not set, it attempts to find 'arm' in PATH or build it from source.
func getArmBinary(t *testing.T) string {
	t.Helper()

	if armBin := os.Getenv("ARM_BIN"); armBin != "" {
		return armBin
	}

	// Try to find 'arm' in PATH
	armBin, err := exec.LookPath("arm")
	if err == nil {
		return armBin
	}

	// Try to build it from source (assumes we're in the repo root)
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

// findRepoRoot finds the root of the armature repository by looking for .git directory.
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

// runCmd runs a git command in the given directory and returns its combined output.
// It panics if the command exits with a non-zero status.
func runCmd(dir string, args ...string) string {
	return runCmdWithEnv(dir, nil, "git", args...)
}

// runCmdWithEnv runs a command in the given directory with environment variables
// and returns its combined output. It panics if the command exits with a non-zero status.
func runCmdWithEnv(dir string, env map[string]string, name string, args ...string) string {
	// #nosec G204 -- test helper invokes git/arm with test-controlled args
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir

	// Inherit existing environment and add/override with test env vars
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

// runCmdSafely runs a command and returns an error if it fails (does not panic).
func runCmdSafely(dir string, env map[string]string, name string, args ...string) error {
	// #nosec G204 -- test helper invokes git/arm with test-controlled args
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir

	// Inherit existing environment and add/override with test env vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	_, err := cmd.CombinedOutput()
	return err
}
