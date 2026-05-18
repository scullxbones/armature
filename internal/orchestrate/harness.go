package orchestrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
)

const defaultHarnessPrompt = "Execute the assigned Armature task in this repository, complete the required code changes, run relevant verification, and exit."

// NewHarnessAdapter creates a harness adapter for the given config.
// Valid adapter names are "claude", "codex", and "devin".
func NewHarnessAdapter(cfg HarnessConfig) (HarnessAdapter, error) {
	switch cfg.Adapter {
	case "claude":
		return &claudeAdapter{cfg: cfg}, nil
	case "codex":
		return &codexAdapter{cfg: cfg}, nil
	case "devin":
		return &devinAdapter{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown harness %q: valid values are claude, codex, devin", cfg.Adapter)
	}
}

// SandboxAvailable checks whether the OS-level sandbox binary is available.
// On macOS it looks for sandbox-exec; on Linux/WSL2 it looks for bwrap.
func SandboxAvailable() (bool, error) {
	if runtime.GOOS == "darwin" {
		if _, err := adapters.LookPath("sandbox-exec"); err == nil {
			return true, nil
		}
		return false, fmt.Errorf("sandbox-exec not found (required on macOS)")
	}
	// Linux / WSL2: bubblewrap
	if _, err := adapters.LookPath("bwrap"); err == nil {
		return true, nil
	}
	return false, fmt.Errorf("bwrap not found — install bubblewrap: apt-get install bubblewrap")
}

// buildSandboxCmd wraps cmdArgs in the OS sandbox restricted to worktreeAbs.
func buildSandboxCmd(worktreeAbs string, cmdArgs []string) []string {
	if runtime.GOOS == "darwin" {
		profile := fmt.Sprintf(
			`(version 1)(allow default)(deny file-write* (subpath "/"))(allow file-write* (subpath "%s"))`,
			worktreeAbs,
		)
		return append([]string{"sandbox-exec", "-p", profile}, cmdArgs...)
	}
	// bwrap: bind-mount root read-only, worktree read-write
	bwrapArgs := []string{
		"bwrap",
		"--ro-bind", "/", "/",
		"--bind", worktreeAbs, worktreeAbs,
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--die-with-parent",
		"--",
	}
	return append(bwrapArgs, cmdArgs...)
}

// invokeProcess launches cmdArgs in workdir, captures stdout+stderr, and returns
// an InvocationResult. In dry-run mode no process is spawned.
func invokeProcess(ctx context.Context, workdir string, cmdArgs []string, dryRun bool) (InvocationResult, error) {
	if dryRun {
		return InvocationResult{Status: ExitSuccess}, nil
	}

	var stdout, stderr strings.Builder
	mw := io.MultiWriter(&stdout, os.Stdout)
	mwErr := io.MultiWriter(&stderr, os.Stderr)

	start := time.Now()
	status, err := adapters.RunProcess(ctx, workdir, cmdArgs, mw, mwErr)
	durationMs := time.Since(start).Milliseconds()

	result := InvocationResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: durationMs,
	}

	switch status {
	case adapters.ProcessClean:
		result.Status = ExitSuccess
		result.ExitCode = 0
	case adapters.ProcessTimeout:
		result.Status = ExitTimeout
	default:
		result.Status = ExitFailure
		if err != nil {
			var exitErr interface{ ExitCode() int }
			if errAs(err, &exitErr) {
				result.ExitCode = exitErr.ExitCode()
			}
		}
	}

	return result, err
}

// errAs is a helper to avoid importing os/exec directly.
// It checks if err satisfies the ExitCode() interface.
func errAs(err error, target *interface{ ExitCode() int }) bool {
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		*target = ec
		return true
	}
	return false
}

// ===== Context helpers =====

// issueCtxKey is a private context key for passing scope information to adapters.
type issueCtxKey struct{}

// issueContext holds the scope paths for a harness invocation.
type issueContext struct {
	Scope        []string
	TaskID       string
	TaskTitle    string
	TaskContract string
}

// WithIssueScope injects scope paths into ctx so harness adapters can read them.
func WithIssueScope(ctx context.Context, scope []string) context.Context {
	return WithIssueContext(ctx, "", "", "", scope)
}

// WithIssueContext injects issue metadata and scope into ctx for harness adapters.
func WithIssueContext(ctx context.Context, taskID, taskTitle, taskContract string, scope []string) context.Context {
	return context.WithValue(ctx, issueCtxKey{}, &issueContext{
		Scope:        scope,
		TaskID:       taskID,
		TaskTitle:    taskTitle,
		TaskContract: taskContract,
	})
}

func issueFromCtx(ctx context.Context) *issueContext {
	if v, ok := ctx.Value(issueCtxKey{}).(*issueContext); ok {
		return v
	}
	return &issueContext{}
}

// validateIssueScope returns an error if scope is empty.
func validateIssueScope(scope []string) error {
	if len(scope) == 0 {
		return fmt.Errorf("issue has no declared scope paths — cannot configure harness sandbox")
	}
	return nil
}

func buildHarnessPrompt(issue *issueContext) string {
	scope := strings.Join(issue.Scope, ", ")
	task := issue.TaskID
	if issue.TaskTitle != "" {
		task = fmt.Sprintf("%s (%s)", issue.TaskID, issue.TaskTitle)
	}
	if strings.TrimSpace(task) == "" {
		task = "unspecified"
	}
	contract := strings.TrimSpace(issue.TaskContract)
	if contract == "" {
		contract = "none provided"
	}
	return fmt.Sprintf("%s Task: %s. Acceptance contract: %s. Scope: %s", defaultHarnessPrompt, task, contract, scope)
}

func buildClaudeLaunchArgs(model, prompt string) []string {
	args := []string{"claude", "--print", "--output-format", "text", "--permission-mode", "dontAsk"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, prompt)
}

func buildCodexLaunchArgs(model, prompt string) []string {
	args := []string{"codex", "exec", "--color", "never"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, prompt)
}

// ===== Claude adapter =====

type claudeAdapter struct{ cfg HarnessConfig }

func (a *claudeAdapter) Name() string { return "claude" }

func (a *claudeAdapter) Run(ctx context.Context, cfg HarnessConfig, opts RunOptions) (CheckResult, error) {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = a.cfg.WorkDir
	}

	if opts.DryRun {
		return CheckResult{Name: "claude", Severity: SeverityInfo, Passed: true, Message: "dry-run: skipped"}, nil
	}

	issue := issueFromCtx(ctx)
	if err := validateIssueScope(issue.Scope); err != nil {
		return CheckResult{Name: "claude", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	if err := writeClaudeSettings(workDir, issue.Scope); err != nil {
		return CheckResult{Name: "claude", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	prompt := buildHarnessPrompt(issue)
	args := buildClaudeLaunchArgs(a.cfg.Model, prompt)

	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return CheckResult{Name: "claude", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}
	sandboxed := buildSandboxCmd(absWork, args)

	inv, err := invokeProcess(ctx, workDir, sandboxed, opts.DryRun)
	result := CheckResult{
		Name:       "claude",
		Severity:   SeverityError,
		Passed:     inv.Status == ExitSuccess,
		Invocation: inv,
	}
	if result.Passed {
		result.Severity = SeverityInfo
		result.Message = "claude invocation succeeded"
	} else {
		result.Message = "claude invocation failed"
	}
	return result, err
}

// writeClaudeSettings writes .claude/settings.json with sandbox permissions.
// The file must NOT include dangerouslySkipPermissions.
func writeClaudeSettings(workdir string, scopePaths []string) error {
	if err := validateIssueScope(scopePaths); err != nil {
		return err
	}
	dir := filepath.Join(workdir, ".claude")
	if err := adapters.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	settings := map[string]any{
		"sandbox": map[string]any{
			"enabled":           true,
			"failIfUnavailable": true,
			"filesystem": map[string]any{
				"allowWrite": scopePaths,
				"denyWrite":  []string{"../"},
			},
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return adapters.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

// ===== Codex adapter =====

type codexAdapter struct{ cfg HarnessConfig }

func (a *codexAdapter) Name() string { return "codex" }

func (a *codexAdapter) Run(ctx context.Context, cfg HarnessConfig, opts RunOptions) (CheckResult, error) {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = a.cfg.WorkDir
	}

	if opts.DryRun {
		return CheckResult{Name: "codex", Severity: SeverityInfo, Passed: true, Message: "dry-run: skipped"}, nil
	}

	issue := issueFromCtx(ctx)
	if err := validateIssueScope(issue.Scope); err != nil {
		return CheckResult{Name: "codex", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	if err := writeCodexConfig(workDir, issue.Scope); err != nil {
		return CheckResult{Name: "codex", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}
	codexHome := filepath.Join(workDir, ".codex-home")
	if err := adapters.MkdirAll(codexHome, 0o755); err != nil {
		return CheckResult{Name: "codex", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	prompt := buildHarnessPrompt(issue)
	args := buildCodexLaunchArgs(a.cfg.Model, prompt)
	args = append([]string{"env", "CODEX_HOME=" + codexHome}, args...)

	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return CheckResult{Name: "codex", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}
	sandboxed := buildSandboxCmd(absWork, args)

	inv, err := invokeProcess(ctx, workDir, sandboxed, opts.DryRun)
	result := CheckResult{
		Name:       "codex",
		Severity:   SeverityError,
		Passed:     inv.Status == ExitSuccess,
		Invocation: inv,
	}
	if result.Passed {
		result.Severity = SeverityInfo
		result.Message = "codex invocation succeeded"
	} else {
		result.Message = "codex invocation failed"
	}
	return result, err
}

// writeCodexConfig writes codex.toml with sandbox permissions.
func writeCodexConfig(workdir string, scopePaths []string) error {
	roots := make([]string, len(scopePaths))
	for i, p := range scopePaths {
		roots[i] = fmt.Sprintf("%q", p)
	}
	toml := fmt.Sprintf(
		"sandbox_mode = \"workspace-write\"\napproval_policy = \"never\"\n[permissions.default.filesystem]\nwritable_roots = [%s]\n",
		strings.Join(roots, ", "),
	)
	return adapters.WriteFile(filepath.Join(workdir, "codex.toml"), []byte(toml), 0o644)
}

// ===== Devin adapter =====

type devinAdapter struct{ cfg HarnessConfig }

func (a *devinAdapter) Name() string { return "devin" }

func (a *devinAdapter) Run(ctx context.Context, cfg HarnessConfig, opts RunOptions) (CheckResult, error) {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = a.cfg.WorkDir
	}

	if opts.DryRun {
		return CheckResult{Name: "devin", Severity: SeverityInfo, Passed: true, Message: "dry-run: skipped"}, nil
	}

	issue := issueFromCtx(ctx)
	if err := validateIssueScope(issue.Scope); err != nil {
		return CheckResult{Name: "devin", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	if err := writeDevinConfig(workDir, issue.Scope); err != nil {
		return CheckResult{Name: "devin", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}

	if a.cfg.Model != "" {
		fmt.Fprintf(os.Stderr, "warning: Devin CLI does not support model selection (model %q ignored)\n", a.cfg.Model)
	}
	args := []string{"devin", "--sandbox", "--permission-mode", "autonomous"}

	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return CheckResult{Name: "devin", Severity: SeverityError, Passed: false, Message: err.Error()}, err
	}
	sandboxed := buildSandboxCmd(absWork, args)

	inv, err := invokeProcess(ctx, workDir, sandboxed, opts.DryRun)
	result := CheckResult{
		Name:       "devin",
		Severity:   SeverityError,
		Passed:     inv.Status == ExitSuccess,
		Invocation: inv,
	}
	if result.Passed {
		result.Severity = SeverityInfo
		result.Message = "devin invocation succeeded"
	} else {
		result.Message = "devin invocation failed"
	}
	return result, err
}

// writeDevinConfig writes .devin/config.json with sandbox permissions.
func writeDevinConfig(workdir string, scopePaths []string) error {
	dir := filepath.Join(workdir, ".devin")
	if err := adapters.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	perms := make([]map[string]string, len(scopePaths))
	for i, p := range scopePaths {
		perms[i] = map[string]string{"allow": fmt.Sprintf("Write(%s)", p)}
	}
	cfg := map[string]any{"permissions": perms}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal devin config: %w", err)
	}
	return adapters.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}
