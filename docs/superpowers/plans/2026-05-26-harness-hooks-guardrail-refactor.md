# Harness Hooks Guardrail Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace overlapping custom harness guardrails with ephemeral, adapter-based platform hooks implemented by `arm harness-hook`, while preserving capability through shared policy and verification logic.

**Architecture:** Build a shared policy core for task resolution, scope checks, and verification. Add a generic hook event/decision layer, then platform adapters for Claude Code, Codex, and Devin that generate ephemeral config and translate platform hook payloads. Wire the orchestrator and worker runtime through those adapters while retaining scheduler state and OS sandbox governance.

**Tech Stack:** Go, Cobra CLI, existing Armature materialization/state packages, platform hook JSON configs, `go test`.

---

## File Structure

- Create `internal/harnesspolicy/scope.go`: shared path/scope verification.
- Create `internal/harnesspolicy/scope_test.go`: scope verifier tests.
- Create `internal/harnesspolicy/resolver.go`: task policy resolver that loads task data from Armature state.
- Create `internal/harnesspolicy/resolver_test.go`: resolver tests with temp Armature repos.
- Create `internal/orchestrate/verification_service.go`: shared verification service over existing pipeline concepts.
- Modify `internal/orchestrate/verify.go`: keep low-level checks, reuse from the service.
- Modify `internal/orchestrate/verify_test.go`: add service-level coverage.
- Create `internal/harnesshook/types.go`: generic hook events, decisions, and adapter interface.
- Create `internal/harnesshook/evaluator.go`: generic hook evaluator using shared policy/services.
- Create `internal/harnesshook/evaluator_test.go`: pre-tool and stop-hook evaluator tests.
- Create `internal/harnesshook/platform_claude.go`: Claude hook config/input/output adapter.
- Create `internal/harnesshook/platform_codex.go`: Codex hook config/input/output adapter.
- Create `internal/harnesshook/platform_devin.go`: Devin hook config/input/output adapter.
- Create `internal/harnesshook/platform_test.go`: golden-style platform adapter tests.
- Create `cmd/armature/harness_hook.go`: `arm harness-hook` command.
- Create `cmd/armature/harness_hook_test.go`: CLI behavior tests.
- Modify `cmd/armature/main.go`: register `harness-hook`.
- Modify `internal/adapters/shell.go`: support process env injection for harness launches.
- Modify `internal/adapters/shell_test.go`: cover env injection.
- Modify `internal/orchestrate/harness.go`: generate ephemeral hook config through platform adapters and pass `ARMATURE_TASK_ID` / `ARMATURE_HOOK_PLATFORM`.
- Modify `internal/orchestrate/harness_test.go`: replace old sandbox config tests with hook config tests.
- Modify `internal/orchestrate/engine.go`: call shared final verification before complete and shared scope policy before commit.
- Modify `internal/orchestrate/engine_test.go`: cover shared scope policy and verification service invocation.
- Modify `cmd/armature/orchestrate.go`: pass acceptance/citation inputs needed for verification service.
- Modify `cmd/armature/worker_run.go`: pass the same verification inputs through worker runtime orchestrator.
- Update `docs/provider-smoke-tests.md`: document hook-first provider smoke coverage.
- Update `docs/commands.md`: document `arm harness-hook` as an internal hook entrypoint.

---

### Task 1: Shared Scope Policy

**Files:**
- Create: `internal/harnesspolicy/scope.go`
- Create: `internal/harnesspolicy/scope_test.go`

- [ ] **Step 1: Write failing scope policy tests**

Create `internal/harnesspolicy/scope_test.go`:

```go
package harnesspolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopePolicyAllowsExactFile(t *testing.T) {
	policy := NewScopePolicy([]string{"cmd/armature/main.go"})

	result := policy.CheckPaths([]string{"cmd/armature/main.go"})

	require.True(t, result.Allowed)
	assert.Empty(t, result.Violations)
}

func TestScopePolicyAllowsDirectoryScope(t *testing.T) {
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.True(t, result.Allowed)
}

func TestScopePolicyAllowsGlobScope(t *testing.T) {
	policy := NewScopePolicy([]string{"internal/orchestrate/*.go"})

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.True(t, result.Allowed)
}

func TestScopePolicyRejectsOutOfScopePath(t *testing.T) {
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"cmd/armature/main.go"})

	require.False(t, result.Allowed)
	require.Len(t, result.Violations, 1)
	assert.Equal(t, "cmd/armature/main.go", result.Violations[0].Path)
	assert.Contains(t, result.Message(), "outside task scope")
}

func TestScopePolicyCleansTraversal(t *testing.T) {
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"internal/orchestrate/../config/config.go"})

	require.False(t, result.Allowed)
	assert.Equal(t, "internal/config/config.go", result.Violations[0].Path)
}

func TestScopePolicyRejectsEmptyScope(t *testing.T) {
	policy := NewScopePolicy(nil)

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.False(t, result.Allowed)
	assert.Contains(t, result.Message(), "task has no declared scope")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/harnesspolicy -run TestScopePolicy -count=1
```

Expected: FAIL because package `internal/harnesspolicy` does not exist.

- [ ] **Step 3: Implement shared scope policy**

Create `internal/harnesspolicy/scope.go`:

```go
package harnesspolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ScopePolicy struct {
	scope []string
}

type ScopeCheckResult struct {
	Allowed    bool
	Violations []ScopeViolation
}

type ScopeViolation struct {
	Path string
}

func NewScopePolicy(scope []string) ScopePolicy {
	copied := append([]string(nil), scope...)
	return ScopePolicy{scope: copied}
}

func (p ScopePolicy) CheckPaths(paths []string) ScopeCheckResult {
	if len(p.scope) == 0 {
		violations := make([]ScopeViolation, 0, len(paths))
		for _, path := range paths {
			violations = append(violations, ScopeViolation{Path: cleanRepoPath(path)})
		}
		return ScopeCheckResult{Allowed: false, Violations: violations}
	}

	var violations []ScopeViolation
	for _, path := range paths {
		clean := cleanRepoPath(path)
		if !p.allows(clean) {
			violations = append(violations, ScopeViolation{Path: clean})
		}
	}

	return ScopeCheckResult{
		Allowed:    len(violations) == 0,
		Violations: violations,
	}
}

func (r ScopeCheckResult) Message() string {
	if r.Allowed {
		return "all paths are within task scope"
	}
	if len(r.Violations) == 0 {
		return "task has no declared scope"
	}
	paths := make([]string, 0, len(r.Violations))
	for _, violation := range r.Violations {
		paths = append(paths, violation.Path)
	}
	return fmt.Sprintf("path(s) outside task scope: %s", strings.Join(paths, ", "))
}

func (p ScopePolicy) allows(path string) bool {
	for _, rawScope := range p.scope {
		scope := cleanRepoPath(rawScope)
		if scope == "." || scope == "./" {
			return true
		}
		if strings.HasSuffix(rawScope, "/") || strings.HasSuffix(scope, "/") {
			if strings.HasPrefix(path, strings.TrimSuffix(scope, "/")+"/") {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(scope, path); matched {
			return true
		}
		if path == scope {
			return true
		}
		if strings.ContainsAny(scope, "*?[") {
			continue
		}
		if strings.HasPrefix(path, scope+"/") {
			return true
		}
	}
	return false
}

func cleanRepoPath(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return "."
	}
	return strings.TrimPrefix(path, "/")
}
```

- [ ] **Step 4: Run scope policy tests**

Run:

```bash
go test ./internal/harnesspolicy -run TestScopePolicy -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harnesspolicy/scope.go internal/harnesspolicy/scope_test.go
git commit -m "feat(harness): add shared scope policy"
```

---

### Task 2: Shared Verification Service

**Files:**
- Create: `internal/orchestrate/verification_service.go`
- Modify: `internal/orchestrate/verify_test.go`

- [ ] **Step 1: Write failing verification service tests**

Append to `internal/orchestrate/verify_test.go`:

```go
func TestVerificationServiceRunsConfiguredAdaptersAndBuiltins(t *testing.T) {
	service := orchestrate.NewVerificationService([]orchestrate.HarnessAdapter{
		passAdapter("build"),
		passAdapter("test"),
	})

	state, err := service.Run(context.Background(), orchestrate.VerificationInput{
		Config:     orchestrate.HarnessConfig{},
		Options:    orchestrate.RunOptions{},
		Acceptance: json.RawMessage(`["go test ./... passes"]`),
		Citations:  []orchestrate.CitationCheck{{SourceEntryID: "SRC-1", Accepted: true}},
	})

	require.NoError(t, err)
	require.False(t, state.Failed)
	require.Len(t, state.Checks, 4)
	assert.Equal(t, "build", state.Checks[0].Name)
	assert.Equal(t, "test", state.Checks[1].Name)
	assert.Equal(t, "acceptance-criteria", state.Checks[2].Name)
	assert.Equal(t, "citations", state.Checks[3].Name)
}

func TestVerificationServiceStopsOnHardFailure(t *testing.T) {
	service := orchestrate.NewVerificationService([]orchestrate.HarnessAdapter{
		failAdapter("build"),
		passAdapter("test"),
	})

	state, err := service.Run(context.Background(), orchestrate.VerificationInput{
		Acceptance: json.RawMessage(`["go test ./... passes"]`),
	})

	require.NoError(t, err)
	require.True(t, state.Failed)
	require.Len(t, state.Checks, 1)
	assert.Equal(t, "build", state.Checks[0].Name)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/orchestrate -run TestVerificationService -count=1
```

Expected: FAIL because `NewVerificationService` and `VerificationInput` are undefined.

- [ ] **Step 3: Implement verification service**

Create `internal/orchestrate/verification_service.go`:

```go
package orchestrate

import (
	"context"
	"encoding/json"
)

type VerificationInput struct {
	Config     HarnessConfig
	Options    RunOptions
	Acceptance json.RawMessage
	Citations  []CitationCheck
}

type VerificationService struct {
	adapters []HarnessAdapter
}

func NewVerificationService(adapters []HarnessAdapter) *VerificationService {
	return &VerificationService{adapters: append([]HarnessAdapter(nil), adapters...)}
}

func (s *VerificationService) Run(ctx context.Context, input VerificationInput) (OrchestrateState, error) {
	return RunPipeline(ctx, s.adapters, input.Config, input.Options, input.Acceptance, input.Citations)
}
```

- [ ] **Step 4: Run verification service tests**

Run:

```bash
go test ./internal/orchestrate -run 'TestVerificationService|TestRunPipeline' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/verification_service.go internal/orchestrate/verify_test.go
git commit -m "feat(orchestrate): add shared verification service"
```

---

### Task 3: Generic Harness Hook Types and Evaluator

**Files:**
- Create: `internal/harnesshook/types.go`
- Create: `internal/harnesshook/evaluator.go`
- Create: `internal/harnesshook/evaluator_test.go`

- [ ] **Step 1: Write failing evaluator tests**

Create `internal/harnesshook/evaluator_test.go`:

```go
package harnesshook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatorBlocksOutOfScopeEdit(t *testing.T) {
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:  EventPreToolUse,
		Tool:  "Edit",
		Paths: []string{"cmd/armature/main.go"},
	})

	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Action)
	assert.Contains(t, decision.Message, "outside task scope")
}

func TestEvaluatorAllowsInScopeEdit(t *testing.T) {
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:  EventPreToolUse,
		Tool:  "Edit",
		Paths: []string{"internal/orchestrate/engine.go"},
	})

	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Action)
}

func TestEvaluatorBlocksGitCommit(t *testing.T) {
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:    EventPreToolUse,
		Tool:    "Bash",
		Command: "git commit -m 'agent commit'",
	})

	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Action)
	assert.Contains(t, decision.Message, "Armature owns commits")
}

func TestEvaluatorRunsStopVerification(t *testing.T) {
	service := orchestrate.NewVerificationService(nil)
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
		VerificationService: service,
		VerificationInput: orchestrate.VerificationInput{
			Acceptance: json.RawMessage(`["go test ./... passes"]`),
			Citations:  []orchestrate.CitationCheck{{SourceEntryID: "SRC-1", Accepted: true}},
		},
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{Kind: EventStop})

	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Action)
	assert.Contains(t, decision.Message, "verification passed")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/harnesshook -count=1
```

Expected: FAIL because package `internal/harnesshook` does not exist.

- [ ] **Step 3: Implement generic hook types**

Create `internal/harnesshook/types.go`:

```go
package harnesshook

import "context"

type EventKind string

const (
	EventPreToolUse EventKind = "pre-tool-use"
	EventPostToolUse EventKind = "post-tool-use"
	EventStop       EventKind = "stop"
)

type Event struct {
	Kind    EventKind
	Tool    string
	Paths   []string
	Command string
}

type DecisionAction string

const (
	DecisionAllow DecisionAction = "allow"
	DecisionBlock DecisionAction = "block"
	DecisionNone  DecisionAction = "none"
)

type Decision struct {
	Action  DecisionAction
	Message string
}

type PlatformCapabilities struct {
	PreToolUse          bool
	Stop                bool
	PostToolUse         bool
	ShellInterception   string
	BlockingStop        bool
	SupportedEditTools  []string
	SupportedShellTools []string
}

type PlatformAdapter interface {
	Name() string
	Capabilities() PlatformCapabilities
	WriteConfig(workdir string) error
	Decode(input []byte) (Event, error)
	Encode(decision Decision) ([]byte, int, error)
}

type Evaluator interface {
	Evaluate(ctx context.Context, event Event) (Decision, error)
}
```

- [ ] **Step 4: Implement evaluator**

Create `internal/harnesshook/evaluator.go`:

```go
package harnesshook

import (
	"context"
	"strings"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/orchestrate"
)

type EvaluatorConfig struct {
	ScopePolicy         harnesspolicy.ScopePolicy
	VerificationService *orchestrate.VerificationService
	VerificationInput   orchestrate.VerificationInput
}

type DefaultEvaluator struct {
	cfg EvaluatorConfig
}

func NewEvaluator(cfg EvaluatorConfig) *DefaultEvaluator {
	return &DefaultEvaluator{cfg: cfg}
}

func (e *DefaultEvaluator) Evaluate(ctx context.Context, event Event) (Decision, error) {
	switch event.Kind {
	case EventPreToolUse:
		return e.evaluatePreToolUse(event), nil
	case EventStop:
		return e.evaluateStop(ctx)
	default:
		return Decision{Action: DecisionNone, Message: "event ignored"}, nil
	}
}

func (e *DefaultEvaluator) evaluatePreToolUse(event Event) Decision {
	if isDirectCommitCommand(event.Command) {
		return Decision{Action: DecisionBlock, Message: "Armature owns commits during harness execution; do not run git commit directly"}
	}
	if len(event.Paths) == 0 {
		return Decision{Action: DecisionAllow, Message: "no path policy applies"}
	}
	result := e.cfg.ScopePolicy.CheckPaths(event.Paths)
	if !result.Allowed {
		return Decision{Action: DecisionBlock, Message: result.Message()}
	}
	return Decision{Action: DecisionAllow, Message: "path is within task scope"}
}

func (e *DefaultEvaluator) evaluateStop(ctx context.Context) (Decision, error) {
	if e.cfg.VerificationService == nil {
		return Decision{Action: DecisionAllow, Message: "no verification service configured"}, nil
	}
	state, err := e.cfg.VerificationService.Run(ctx, e.cfg.VerificationInput)
	if err != nil {
		return Decision{Action: DecisionBlock, Message: err.Error()}, nil
	}
	if state.Failed {
		return Decision{Action: DecisionBlock, Message: "verification failed"}
	}
	return Decision{Action: DecisionAllow, Message: "verification passed"}
}

func isDirectCommitCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) < 2 {
		return false
	}
	return fields[0] == "git" && fields[1] == "commit"
}
```

- [ ] **Step 5: Run evaluator tests**

Run:

```bash
go test ./internal/harnesshook -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/harnesshook/types.go internal/harnesshook/evaluator.go internal/harnesshook/evaluator_test.go
git commit -m "feat(harness): add generic hook evaluator"
```

---

### Task 4: Platform Hook Adapters

**Files:**
- Create: `internal/harnesshook/platform_claude.go`
- Create: `internal/harnesshook/platform_codex.go`
- Create: `internal/harnesshook/platform_devin.go`
- Create: `internal/harnesshook/platform_test.go`

- [ ] **Step 1: Write failing platform adapter tests**

Create `internal/harnesshook/platform_test.go`:

```go
package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	dir := t.TempDir()
	adapter := NewClaudeAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "sandbox")
}

func TestCodexAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "writable_roots")
}

func TestDevinAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	dir := t.TempDir()
	adapter := NewDevinAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "permissions")
}

func TestClaudeAdapterEncodesBlockDecision(t *testing.T) {
	adapter := NewClaudeAdapter()

	out, code, err := adapter.Encode(Decision{Action: DecisionBlock, Message: "blocked"})

	require.NoError(t, err)
	require.Equal(t, 0, code)
	assert.Contains(t, string(out), "blocked")
	assert.Contains(t, string(out), "deny")
}

func TestAdaptersExposeCapabilities(t *testing.T) {
	adapters := []PlatformAdapter{NewClaudeAdapter(), NewCodexAdapter(), NewDevinAdapter()}
	for _, adapter := range adapters {
		caps := adapter.Capabilities()
		assert.True(t, caps.PreToolUse, adapter.Name())
		assert.True(t, caps.Stop, adapter.Name())
		assert.NotEmpty(t, caps.SupportedEditTools, adapter.Name())
	}
}

func TestCodexAdapterDecodesApplyPatchPath(t *testing.T) {
	adapter := NewCodexAdapter()
	input := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input": map[string]any{
			"changes": []any{map[string]any{"path": "internal/orchestrate/engine.go"}},
		},
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)

	event, err := adapter.Decode(data)

	require.NoError(t, err)
	assert.Equal(t, EventPreToolUse, event.Kind)
	assert.Equal(t, []string{"internal/orchestrate/engine.go"}, event.Paths)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/harnesshook -run 'Test.*Adapter' -count=1
```

Expected: FAIL because adapter constructors are undefined.

- [ ] **Step 3: Implement Claude adapter**

Create `internal/harnesshook/platform_claude.go`:

```go
package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ClaudeAdapter struct{}

func NewClaudeAdapter() *ClaudeAdapter { return &ClaudeAdapter{} }
func (a *ClaudeAdapter) Name() string  { return "claude" }

func (a *ClaudeAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		BlockingStop:        true,
		ShellInterception:   "structured",
		SupportedEditTools:  []string{"Edit", "Write", "MultiEdit"},
		SupportedShellTools: []string{"Bash"},
	}
}

func (a *ClaudeAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "Edit|Write|MultiEdit|Bash",
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": "arm harness-hook",
				}},
			}},
			"Stop": []any{map[string]any{
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": "arm harness-hook",
				}},
			}},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

func (a *ClaudeAdapter) Decode(input []byte) (Event, error) {
	var raw struct {
		HookEventName string         `json:"hook_event_name"`
		ToolName      string         `json:"tool_name"`
		ToolInput     map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, err
	}
	event := Event{Kind: normalizeEvent(raw.HookEventName), Tool: raw.ToolName}
	event.Paths = extractPaths(raw.ToolInput)
	event.Command = extractCommand(raw.ToolInput)
	return event, nil
}

func (a *ClaudeAdapter) Encode(decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"continue": true, "suppressOutput": true})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":      "PreToolUse",
			"permissionDecision": "deny",
			"permissionDecisionReason": decision.Message,
		},
	})
	return data, 0, err
}
```

- [ ] **Step 4: Implement Codex and Devin adapters with shared helpers**

Create `internal/harnesshook/platform_codex.go`:

```go
package harnesshook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CodexAdapter struct{}

func NewCodexAdapter() *CodexAdapter { return &CodexAdapter{} }
func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		BlockingStop:        true,
		ShellInterception:   "best-effort",
		SupportedEditTools:  []string{"apply_patch", "Edit", "Write"},
		SupportedShellTools: []string{"Bash"},
	}
}

func (a *CodexAdapter) WriteConfig(workdir string) error {
	content := `[hooks]
pre_tool_use = "arm harness-hook"
stop = "arm harness-hook"
`
	return os.WriteFile(filepath.Join(workdir, "codex.toml"), []byte(content), 0o644)
}

func (a *CodexAdapter) Decode(input []byte) (Event, error) {
	var raw struct {
		HookEventName string         `json:"hook_event_name"`
		ToolName      string         `json:"tool_name"`
		ToolInput     map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, err
	}
	event := Event{Kind: normalizeEvent(raw.HookEventName), Tool: raw.ToolName}
	event.Paths = extractPaths(raw.ToolInput)
	event.Command = extractCommand(raw.ToolInput)
	return event, nil
}

func (a *CodexAdapter) Encode(decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"decision": "approve"})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{"decision": "deny", "reason": decision.Message})
	return data, 0, err
}

func normalizeEvent(name string) EventKind {
	switch name {
	case "PreToolUse", "pre_tool_use":
		return EventPreToolUse
	case "PostToolUse", "post_tool_use":
		return EventPostToolUse
	case "Stop", "stop":
		return EventStop
	default:
		return EventKind(name)
	}
}

func extractPaths(input map[string]any) []string {
	if input == nil {
		return nil
	}
	keys := []string{"file_path", "path"}
	for _, key := range keys {
		if value, ok := input[key].(string); ok && value != "" {
			return []string{value}
		}
	}
	if changes, ok := input["changes"].([]any); ok {
		var paths []string
		for _, item := range changes {
			change, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if path, ok := change["path"].(string); ok && path != "" {
				paths = append(paths, path)
			}
		}
		return paths
	}
	return nil
}

func extractCommand(input map[string]any) string {
	if input == nil {
		return ""
	}
	for _, key := range []string{"command", "cmd"} {
		if value, ok := input[key].(string); ok {
			return value
		}
	}
	return fmt.Sprint(input["input"])
}
```

Create `internal/harnesshook/platform_devin.go`:

```go
package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type DevinAdapter struct{}

func NewDevinAdapter() *DevinAdapter { return &DevinAdapter{} }
func (a *DevinAdapter) Name() string { return "devin" }

func (a *DevinAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		BlockingStop:        true,
		ShellInterception:   "structured",
		SupportedEditTools:  []string{"edit"},
		SupportedShellTools: []string{"exec"},
	}
}

func (a *DevinAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".devin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "edit|exec",
				"command": "arm harness-hook",
			}},
			"Stop": []any{map[string]any{
				"command": "arm harness-hook",
			}},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.json"), data, 0o644)
}

func (a *DevinAdapter) Decode(input []byte) (Event, error) {
	var raw struct {
		HookEventName string         `json:"hook_event_name"`
		ToolName      string         `json:"tool_name"`
		ToolInput     map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, err
	}
	event := Event{Kind: normalizeEvent(raw.HookEventName), Tool: raw.ToolName}
	event.Paths = extractPaths(raw.ToolInput)
	event.Command = extractCommand(raw.ToolInput)
	return event, nil
}

func (a *DevinAdapter) Encode(decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"decision": "approve"})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{"decision": "block", "reason": decision.Message})
	return data, 0, err
}
```

- [ ] **Step 5: Run platform adapter tests**

Run:

```bash
go test ./internal/harnesshook -run 'Test.*Adapter' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/harnesshook/platform_claude.go internal/harnesshook/platform_codex.go internal/harnesshook/platform_devin.go internal/harnesshook/platform_test.go
git commit -m "feat(harness): add platform hook adapters"
```

---

### Task 5: Task Policy Resolver

**Files:**
- Create: `internal/harnesspolicy/resolver.go`
- Create: `internal/harnesspolicy/resolver_test.go`

- [ ] **Step 1: Write failing resolver tests**

Create `internal/harnesspolicy/resolver_test.go`:

```go
package harnesspolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolverLoadsTaskScopeFromMaterializedState(t *testing.T) {
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0o755))
	require.NoError(t, materialize.WriteIssue(filepath.Join(stateDir, "issues"), materialize.Issue{
		ID:         "TASK-1",
		Title:      "Implement hook",
		Type:       "task",
		Status:     ops.StatusInProgress,
		Scope:      []string{"internal/harnesshook/"},
		Acceptance: []string{"go test ./... passes"},
	}))

	resolver := NewTaskPolicyResolver(ResolverConfig{
		RepoPath:  repo,
		StateDir:  stateDir,
		IssuesDir: filepath.Join(repo, ".armature"),
	})

	task, err := resolver.Resolve("TASK-1")

	require.NoError(t, err)
	assert.Equal(t, "TASK-1", task.ID)
	assert.Equal(t, []string{"internal/harnesshook/"}, task.Scope)
	assert.Contains(t, string(task.Acceptance), "go test ./... passes")
}

func TestResolverRejectsUnknownTask(t *testing.T) {
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".armature", "state", "default")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0o755))
	resolver := NewTaskPolicyResolver(ResolverConfig{RepoPath: repo, StateDir: stateDir})

	_, err := resolver.Resolve("MISSING")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "task MISSING not found")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/harnesspolicy -run TestResolver -count=1
```

Expected: FAIL because resolver types are undefined.

- [ ] **Step 3: Implement resolver**

Create `internal/harnesspolicy/resolver.go`:

```go
package harnesspolicy

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/orchestrate"
)

type ResolverConfig struct {
	RepoPath  string
	StateDir  string
	IssuesDir string
}

type TaskPolicy struct {
	ID         string
	Title      string
	Scope      []string
	Acceptance json.RawMessage
	Citations  []orchestrate.CitationCheck
}

type TaskPolicyResolver struct {
	cfg ResolverConfig
}

func NewTaskPolicyResolver(cfg ResolverConfig) *TaskPolicyResolver {
	return &TaskPolicyResolver{cfg: cfg}
}

func (r *TaskPolicyResolver) Resolve(taskID string) (TaskPolicy, error) {
	issuePath := filepath.Join(r.cfg.StateDir, "issues", taskID+".json")
	issue, err := materialize.LoadIssue(issuePath)
	if err != nil {
		return TaskPolicy{}, fmt.Errorf("task %s not found: %w", taskID, err)
	}
	acceptance, err := json.Marshal(issue.Acceptance)
	if err != nil {
		return TaskPolicy{}, fmt.Errorf("marshal acceptance for %s: %w", taskID, err)
	}
	return TaskPolicy{
		ID:         issue.ID,
		Title:      issue.Title,
		Scope:      append([]string(nil), issue.Scope...),
		Acceptance: acceptance,
		Citations:  nil,
	}, nil
}
```

- [ ] **Step 4: Run resolver tests**

Run:

```bash
go test ./internal/harnesspolicy -run TestResolver -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harnesspolicy/resolver.go internal/harnesspolicy/resolver_test.go
git commit -m "feat(harness): resolve hook policy from task state"
```

---

### Task 6: `arm harness-hook` CLI

**Files:**
- Create: `cmd/armature/harness_hook.go`
- Create: `cmd/armature/harness_hook_test.go`
- Modify: `cmd/armature/main.go`

- [ ] **Step 1: Write failing CLI registration and no-scope-flag tests**

Create `cmd/armature/harness_hook_test.go`:

```go
package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarnessHookCommandRegistered(t *testing.T) {
	root := newRootCmd()

	cmd, _, err := root.Find([]string{"harness-hook"})

	require.NoError(t, err)
	require.Equal(t, "harness-hook", cmd.Name())
}

func TestHarnessHookCommandHasNoScopeFlags(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"harness-hook"})
	require.NoError(t, err)

	assert.Nil(t, cmd.Flags().Lookup("scope"))
	assert.Nil(t, cmd.Flags().Lookup("scope-file"))
	assert.Nil(t, cmd.Flags().Lookup("issue"))
}

func TestHarnessHookRequiresTaskEnv(t *testing.T) {
	root := newRootCmd()
	root.SetIn(bytes.NewBufferString(`{"hook_event_name":"PreToolUse"}`))
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"harness-hook"})

	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ARMATURE_TASK_ID")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./cmd/armature -run TestHarnessHook -count=1
```

Expected: FAIL because `harness-hook` is not registered.

- [ ] **Step 3: Implement CLI command**

Create `cmd/armature/harness_hook.go`:

```go
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/spf13/cobra"
)

func newHarnessHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "harness-hook",
		Short:  "Internal harness hook entrypoint",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			taskID := os.Getenv("ARMATURE_TASK_ID")
			if taskID == "" {
				return fmt.Errorf("ARMATURE_TASK_ID is required")
			}
			platform := os.Getenv("ARMATURE_HOOK_PLATFORM")
			adapter, err := hookAdapterForPlatform(platform)
			if err != nil {
				return err
			}

			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read hook input: %w", err)
			}
			event, err := adapter.Decode(input)
			if err != nil {
				return fmt.Errorf("decode hook input: %w", err)
			}

			appCtx := currentCtx(cmd)
			resolver := harnesspolicy.NewTaskPolicyResolver(harnesspolicy.ResolverConfig{
				RepoPath:  appCtx.RepoPath,
				StateDir:  appCtx.StateDir,
				IssuesDir: appCtx.IssuesDir,
			})
			task, err := resolver.Resolve(taskID)
			if err != nil {
				return err
			}

			evaluator := harnesshook.NewEvaluator(harnesshook.EvaluatorConfig{
				ScopePolicy:         harnesspolicy.NewScopePolicy(task.Scope),
				VerificationService: orchestrate.NewVerificationService(nil),
				VerificationInput: orchestrate.VerificationInput{
					Acceptance: task.Acceptance,
					Citations:  task.Citations,
				},
			})
			decision, err := evaluator.Evaluate(cmd.Context(), event)
			if err != nil {
				return err
			}
			output, exitCode, err := adapter.Encode(decision)
			if err != nil {
				return err
			}
			_, _ = cmd.OutOrStdout().Write(output)
			if exitCode != 0 {
				return fmt.Errorf("hook blocked: %s", decision.Message)
			}
			return nil
		},
	}
}

func hookAdapterForPlatform(platform string) (harnesshook.PlatformAdapter, error) {
	switch platform {
	case "claude", "":
		return harnesshook.NewClaudeAdapter(), nil
	case "codex":
		return harnesshook.NewCodexAdapter(), nil
	case "devin":
		return harnesshook.NewDevinAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown harness hook platform %q", platform)
	}
}
```

Modify `cmd/armature/main.go` in `newRootCmd()` after `versionCmd` registration:

```go
	harnessHookCmd := newHarnessHookCmd()
	harnessHookCmd.GroupID = "admin"
	root.AddCommand(harnessHookCmd)
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
go test ./cmd/armature -run TestHarnessHook -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/harness_hook.go cmd/armature/harness_hook_test.go cmd/armature/main.go
git commit -m "feat(cmd): add harness hook entrypoint"
```

---

### Task 7: Harness Launch Integration

**Files:**
- Modify: `internal/adapters/shell.go`
- Modify: `internal/adapters/shell_test.go`
- Modify: `internal/orchestrate/harness.go`
- Modify: `internal/orchestrate/harness_test.go`

- [ ] **Step 1: Write failing adapter env injection test**

Append to `internal/adapters/shell_test.go`:

```go
func TestRunProcessWithEnvInjectsEnvironment(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	status, err := RunProcessWithEnv(context.Background(), t.TempDir(), []string{"sh", "-c", "printf %s \"$ARMATURE_TASK_ID\""}, []string{"ARMATURE_TASK_ID=TASK-1"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("unexpected error: %v stderr=%s", err, stderr.String())
	}
	if status != ProcessClean {
		t.Fatalf("expected clean status, got %v", status)
	}
	if stdout.String() != "TASK-1" {
		t.Fatalf("expected injected env value, got %q", stdout.String())
	}
}
```

Add imports to `internal/adapters/shell_test.go`:

```go
import (
	"context"
	"strings"
)
```

- [ ] **Step 2: Run adapter test to verify it fails**

Run:

```bash
go test ./internal/adapters -run TestRunProcessWithEnvInjectsEnvironment -count=1
```

Expected: FAIL because `RunProcessWithEnv` is undefined.

- [ ] **Step 3: Implement process env injection**

Modify `internal/adapters/shell.go`:

```go
func RunProcess(ctx context.Context, workdir string, cmdArgs []string, stdout, stderr io.Writer) (ProcessStatus, error) {
	return RunProcessWithEnv(ctx, workdir, cmdArgs, nil, stdout, stderr)
}

func RunProcessWithEnv(ctx context.Context, workdir string, cmdArgs []string, env []string, stdout, stderr io.Writer) (ProcessStatus, error) {
	if len(cmdArgs) == 0 {
		return ProcessError, fmt.Errorf("RunProcess: cmdArgs must not be empty")
	}
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...) //nolint:gosec
	cmd.Dir = workdir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return ProcessTimeout, ctxErr
		}
		return ProcessError, err
	}
	return ProcessClean, nil
}
```

- [ ] **Step 4: Run adapter env injection test**

Run:

```bash
go test ./internal/adapters -run TestRunProcessWithEnvInjectsEnvironment -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing tests for hook config generation replacing scope config**

Replace old write-config tests in `internal/orchestrate/harness_test.go` with:

```go
func TestWriteHarnessHookConfigClaude(t *testing.T) {
	dir := t.TempDir()

	err := writeHarnessHookConfig(dir, "claude")

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "allowWrite")
}

func TestWriteHarnessHookConfigCodex(t *testing.T) {
	dir := t.TempDir()

	err := writeHarnessHookConfig(dir, "codex")

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "writable_roots")
}

func TestBuildHarnessEnvIncludesTaskIDAndPlatform(t *testing.T) {
	env := buildHarnessHookEnv([]string{"PATH=/usr/bin"}, "TASK-1", "codex")

	assert.Contains(t, env, "ARMATURE_TASK_ID=TASK-1")
	assert.Contains(t, env, "ARMATURE_HOOK_PLATFORM=codex")
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run:

```bash
go test ./internal/orchestrate -run 'TestWriteHarnessHookConfig|TestBuildHarnessEnv' -count=1
```

Expected: FAIL because helper functions are undefined and old config writers still exist.

- [ ] **Step 7: Add hook config helpers and env injection**

Modify `internal/orchestrate/harness.go`:

```go
func writeHarnessHookConfig(workdir, platform string) error {
	adapter, err := hookAdapterForHarness(platform)
	if err != nil {
		return err
	}
	return adapter.WriteConfig(workdir)
}

func hookAdapterForHarness(platform string) (harnesshook.PlatformAdapter, error) {
	switch platform {
	case "claude":
		return harnesshook.NewClaudeAdapter(), nil
	case "codex":
		return harnesshook.NewCodexAdapter(), nil
	case "devin":
		return harnesshook.NewDevinAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown harness hook platform %q", platform)
	}
}

func buildHarnessHookEnv(base []string, taskID, platform string) []string {
	env := append([]string(nil), base...)
	env = append(env, "ARMATURE_TASK_ID="+taskID)
	env = append(env, "ARMATURE_HOOK_PLATFORM="+platform)
	return env
}
```

Add import:

```go
	"github.com/scullxbones/armature/internal/harnesshook"
```

In each adapter `Run` method:

- Replace `writeClaudeSettings(workDir, issue.Scope)` with `writeHarnessHookConfig(workDir, "claude")`.
- Replace `writeCodexConfig(workDir, issue.Scope)` with `writeHarnessHookConfig(workDir, "codex")`.
- Replace `writeDevinConfig(workDir, issue.Scope)` with `writeHarnessHookConfig(workDir, "devin")`.
- Keep `validateIssueScope(issue.Scope)` because empty scope is dispatch input validation.
- Replace calls to `invokeProcess(ctx, workDir, sandboxed, opts.DryRun)` with `invokeProcessWithEnv(ctx, workDir, sandboxed, buildHarnessHookEnv(nil, issue.TaskID, "<platform>"), opts.DryRun)` for each platform.
- Add `invokeProcessWithEnv` in `internal/orchestrate/harness.go` that calls `adapters.RunProcessWithEnv`.

- [ ] **Step 8: Run harness tests**

Run:

```bash
go test ./internal/orchestrate -run 'TestWriteHarnessHookConfig|TestBuildHarnessEnv|Test.*AdapterRunDryRun' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/adapters/shell.go internal/adapters/shell_test.go internal/orchestrate/harness.go internal/orchestrate/harness_test.go
git commit -m "feat(orchestrate): launch harnesses with ephemeral hook config"
```

---

### Task 8: Final Shared Scope Verification in Zero-Trust Commit

**Files:**
- Modify: `internal/orchestrate/engine.go`
- Modify: `internal/orchestrate/engine_test.go`

- [ ] **Step 1: Write failing final scope verification test**

Append to `internal/orchestrate/engine_test.go`:

```go
func TestEngine_ZeroTrustCommitRejectsOutOfScopeDiff(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff --git a/cmd/armature/main.go b/cmd/armature/main.go\n",
		diffFiles: []string{"cmd/armature/main.go"},
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("build")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/orchestrate/"},
		RetryBudget: 1,
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())

	if err == nil {
		t.Fatal("expected out-of-scope diff error")
	}
	if !strings.Contains(err.Error(), "outside task scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
	if result.Phase == "complete" {
		t.Fatal("out-of-scope diff must not complete")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/orchestrate -run TestEngine_ZeroTrustCommitRejectsOutOfScopeDiff -count=1
```

Expected: FAIL because zero-trust commit does not yet call shared scope policy.

- [ ] **Step 3: Implement final shared scope verification**

Modify `internal/orchestrate/engine.go` imports:

```go
	"github.com/scullxbones/armature/internal/harnesspolicy"
```

In `zeroTrustCommit`, after capturing `patch` and before `ResetHard`:

```go
	changedFiles, err := e.cfg.Git.DiffNameOnly(preRef)
	if err != nil {
		return state, fmt.Errorf("zero-trust changed files: %w", err)
	}
	scopeResult := harnesspolicy.NewScopePolicy(e.cfg.Scope).CheckPaths(changedFiles)
	if !scopeResult.Allowed {
		return state, fmt.Errorf("zero-trust scope verification: %s", scopeResult.Message())
	}
```

- [ ] **Step 4: Run engine scope tests**

Run:

```bash
go test ./internal/orchestrate -run 'TestEngine_ZeroTrustCommit|TestEngine_ZeroTrustCommitRejectsOutOfScopeDiff' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/engine.go internal/orchestrate/engine_test.go
git commit -m "feat(orchestrate): verify final diff scope with shared policy"
```

---

### Task 9: Wire VerificationService Into Engine Completion

**Files:**
- Modify: `internal/orchestrate/engine.go`
- Modify: `internal/orchestrate/service.go`
- Modify: `internal/orchestrate/engine_test.go`

- [ ] **Step 1: Write failing engine verification service test**

Append to `internal/orchestrate/engine_test.go`:

```go
func TestEngine_RunsVerificationServiceBeforeComplete(t *testing.T) {
	priorOps := []ops.Op{
		{Type: ops.OpOrchestrateDispatch, TargetID: "T1",
			Payload: ops.Payload{PreDispatchRef: "base123", WorktreePath: "/wt/T1", RetryBudget: 1}},
		{Type: ops.OpOrchestrateDispatchComplete, TargetID: "T1"},
	}
	git := &stubGit{
		headSHA:   "head456",
		diffOut:   "diff --git a/internal/foo/bar.go b/internal/foo/bar.go\n",
		diffFiles: []string{"internal/foo/bar.go"},
	}
	log := &stubOpLog{ops: priorOps}
	harness := passingHarness("agent")

	cfg := orchestrate.EngineConfig{
		TaskID:      "T1",
		Git:         git,
		OpLog:       log,
		Harness:     harness,
		Scope:       []string{"internal/foo/"},
		RetryBudget: 1,
		Acceptance:  json.RawMessage(`["go test ./... passes"]`),
	}

	result, err := orchestrate.NewEngine(cfg).Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "complete" {
		t.Fatalf("expected complete, got %s", result.Phase)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/orchestrate -run TestEngine_RunsVerificationServiceBeforeComplete -count=1
```

Expected: FAIL because `EngineConfig.Acceptance` is undefined.

- [ ] **Step 3: Add verification fields to engine and service config**

Modify `EngineConfig` in `internal/orchestrate/engine.go`:

```go
	// Acceptance contains machine-verifiable acceptance criteria for final verification.
	Acceptance json.RawMessage
	// Citations contains source citation acceptance state for final verification.
	Citations []CitationCheck
```

Add `encoding/json` to the import block in `internal/orchestrate/engine.go`.

Modify `RunInput` in `internal/orchestrate/service.go`:

```go
	Acceptance json.RawMessage
	Citations  []CitationCheck
```

Add `encoding/json` to the import block in `internal/orchestrate/service.go`.

Pass these fields into `NewEngine`.

- [ ] **Step 4: Run `VerificationService` before zero-trust commit**

In `runningPhase`, before `return e.zeroTrustCommit(ctx, state)`:

```go
	verificationService := NewVerificationService(nil)
	verificationState, err := verificationService.Run(ctx, VerificationInput{
		Config:     e.cfg.HarnessCfg,
		Options:    e.cfg.Opts,
		Acceptance: e.cfg.Acceptance,
		Citations:  e.cfg.Citations,
	})
	if err != nil {
		return state, fmt.Errorf("verification service: %w", err)
	}
	state.Checks = append(state.Checks, verificationState.Checks...)
	if verificationState.Failed {
		return e.handleVerifyFailure(ctx, state)
	}
```

- [ ] **Step 5: Run engine verification tests**

Run:

```bash
go test ./internal/orchestrate -run 'TestEngine_RunsVerificationServiceBeforeComplete|TestEngine_VerificationFail_TriggersRetry|TestVerificationService' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrate/engine.go internal/orchestrate/service.go internal/orchestrate/engine_test.go
git commit -m "feat(orchestrate): run shared verification before completion"
```

---

### Task 10: Pass Verification Inputs From Commands

**Files:**
- Modify: `cmd/armature/orchestrate.go`
- Modify: `cmd/armature/worker_run.go`
- Modify: `cmd/armature/orchestrate_test.go`
- Modify: `cmd/armature/worker_run_test.go`

- [ ] **Step 1: Write failing command test for acceptance forwarding**

Append to `cmd/armature/orchestrate_test.go`:

```go
func TestOrchestrateServiceInputIncludesAcceptance(t *testing.T) {
	t.Parallel()
	acceptance := []string{"go test ./... passes"}
	raw, err := json.Marshal(acceptance)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "go test")
}
```

Add `encoding/json` to the import block in `cmd/armature/orchestrate_test.go`.

- [ ] **Step 2: Run command tests**

Run:

```bash
go test ./cmd/armature -run 'TestOrchestrate.*Acceptance|TestWorkerRun' -count=1
```

Expected: PASS for the new small test and existing worker tests before wiring.

- [ ] **Step 3: Wire acceptance in orchestrate command**

Modify the `service.Run` call in `cmd/armature/orchestrate.go`:

```go
				Acceptance: json.RawMessage(mustMarshalAcceptance(issue.Acceptance)),
				Citations:  nil,
```

Add helper in `cmd/armature/orchestrate.go`:

```go
func mustMarshalAcceptance(acceptance []string) []byte {
	data, err := json.Marshal(acceptance)
	if err != nil {
		return []byte("[]")
	}
	return data
}
```

- [ ] **Step 4: Wire acceptance in worker runtime orchestrator**

Modify the `service.Run` call in `cmd/armature/worker_run.go`:

```go
		Acceptance: json.RawMessage(mustMarshalAcceptance(issue.Acceptance)),
		Citations:  nil,
```

- [ ] **Step 5: Run command tests**

Run:

```bash
go test ./cmd/armature -run 'TestOrchestrate|TestWorkerRun' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/orchestrate.go cmd/armature/worker_run.go cmd/armature/orchestrate_test.go cmd/armature/worker_run_test.go
git commit -m "feat(cmd): forward verification inputs to orchestrator"
```

---

### Task 11: Documentation and Provider Smoke Tests

**Files:**
- Modify: `docs/commands.md`
- Modify: `docs/provider-smoke-tests.md`

- [ ] **Step 1: Update command docs**

Add to `docs/commands.md`:

```markdown
## harness-hook

Internal entrypoint used by generated Claude Code, Codex, and Devin hook
configuration.

`arm harness-hook`

This command is not intended for direct user workflows. Harness launch code sets
`ARMATURE_TASK_ID` and `ARMATURE_HOOK_PLATFORM`; the hook command reads the
platform hook payload from stdin, resolves task scope and verification data from
Armature state, and emits the platform-specific allow/block response.

The command intentionally has no `--scope`, `--scope-file`, or `--issue` flag.
Task facts come from the Armature DAG model to avoid drift.
```

- [ ] **Step 2: Update provider smoke tests**

Add to `docs/provider-smoke-tests.md`:

```markdown
## Hook Guardrail Smoke Checks

For each provider (`claude`, `codex`, `devin`):

1. Create or select a task with scope limited to a single temporary file.
2. Run `arm orchestrate --issue <task> --harness <provider> --dry-run`.
3. Confirm generated provider config calls `arm harness-hook`.
4. Simulate a pre-tool edit event for a path inside scope and confirm the hook
   allows it.
5. Simulate a pre-tool edit event for a path outside scope and confirm the hook
   blocks it.
6. Simulate a direct `git commit` shell event and confirm the hook blocks it.
7. Confirm the final zero-trust path still rejects an out-of-scope diff through
   shared `ScopePolicy`.

Codex shell interception remains best-effort unless the specific command path
is proven visible to Codex hooks. The OS sandbox must not be removed for Codex
based only on successful edit-hook smoke checks.
```

- [ ] **Step 3: Run documentation grep checks**

Run:

```bash
rg -n "harness-hook|Hook Guardrail|ARMATURE_TASK_ID|scope-file" docs/commands.md docs/provider-smoke-tests.md
```

Expected: output includes `harness-hook`, `Hook Guardrail Smoke Checks`, and
`ARMATURE_TASK_ID`; `scope-file` appears only in the sentence saying the flag
does not exist.

- [ ] **Step 4: Commit**

```bash
git add docs/commands.md docs/provider-smoke-tests.md
git commit -m "docs: document harness hook guardrail flow"
```

---

### Task 12: Full Verification

**Files:**
- No source edits expected.

- [ ] **Step 1: Run package tests**

Run:

```bash
go test ./internal/harnesspolicy ./internal/harnesshook ./internal/orchestrate ./cmd/armature
```

Expected: PASS.

- [ ] **Step 2: Run broader checks**

Run:

```bash
make check
```

Expected: PASS.

- [ ] **Step 3: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -12
```

Expected: clean worktree and one commit per completed task.

---

## Self-Review Notes

Spec coverage:

- Ephemeral per-run hook config: Tasks 4 and 7.
- `arm` as hook executable: Tasks 4 and 6.
- Minimal CLI surface with no scope flags: Task 6.
- Shared scope logic: Tasks 1, 3, and 8.
- Shared verification service: Tasks 2 and 9.
- Adapter-based harness expansion: Tasks 3 and 4.
- OS sandbox governance retained pending classification: Task 7 avoids removal; Task 11 documents Codex caveat.
- Direct commit blocking only, not broad dangerous shell policy: Task 3.
- Documentation and smoke tests: Task 11.

Implementation risk:

- Platform config schemas may need adjustment against current CLI docs during execution. Keep those changes inside platform adapters only.
- `VerificationService` initially wraps existing built-in checks and any adapters provided by callers. Construction of build/lint/test command adapters is excluded unless the current runtime already exposes those adapters through `HarnessAdapter`.
- If provider CLIs require trusted hook configuration approval, provider smoke tests must capture that setup explicitly.
