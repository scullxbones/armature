# Trellis E1: Bootstrap Solo Workflow — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete solo-workflow CLI (`trls`) for single-branch mode — from op log engine through materialization, ready-task computation, claiming, context assembly, decomposition, hooks, and error diagnostics — so that the dogfood ceremony can load E2/E3/E4 into Trellis itself.

**Architecture:** Functional core / hexagonal architecture. Pure functions for all DAG, materialization, ready-computation, and context-assembly logic (no I/O). Thin boundary adapters for git and filesystem. All coordination state is append-only JSONL op logs; materialized state files are local-only caches derived from logs. Single-branch mode stores `.issues/` on the code branch directly.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra` (CLI), `github.com/leanovate/gopter` (property tests), `github.com/stretchr/testify` (assertions), `github.com/google/uuid` (worker IDs). No external deps for core logic.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E1 section)
**Architecture doc:** `docs/architecture.md`
**PRD:** `docs/trellis-prd.md`

---

## Story Dependency Order

```
S7: CLI Entry Point + Init + Worker Identity  [no deps — sets up project structure]
S1: Data Model & Op Log Engine                [no deps — defines core types]
S2: Materialization Engine                    [depends on S1]
S6: Status Transitions & Core Ops             [depends on S1 — parallel with S2]
S3: Ready Task Computation                    [depends on S2]
S4: Claim System                              [depends on S3]
S5: Context Assembly                          [depends on S2]
S8: Decomposition Workflow + Validate         [depends on S1, S2]
S9: Pre-Transition Hooks                      [depends on S4, S6]
S10: Structured Error Diagnostics             [cross-cutting]
S11: SKILL.md + plan.json                     [depends on S3-S9]
```

---

## File Structure

### New Files (grouped by package)

| Package | File | Responsibility |
|---------|------|---------------|
| `cmd/trellis` | `main.go` | Cobra root command, global flags (`--debug`, `--format`) |
| `cmd/trellis` | `version.go` | `trls version` — prints module version |
| `cmd/trellis` | `init.go` | `trls init` — single-branch setup, config.json generation |
| `cmd/trellis` | `worker_init.go` | `trls worker-init` / `trls worker-init --check` |
| `cmd/trellis` | `materialize.go` | `trls materialize` — standalone CLI for materialization |
| `cmd/trellis` | `ready.go` | `trls ready` — show ready tasks |
| `cmd/trellis` | `claim.go` | `trls claim` — claim a task |
| `cmd/trellis` | `heartbeat.go` | `trls heartbeat` — heartbeat for active claim |
| `cmd/trellis` | `transition.go` | `trls transition` — status transitions |
| `cmd/trellis` | `create.go` | `trls create` — create new node |
| `cmd/trellis` | `note.go` | `trls note` — add note to issue |
| `cmd/trellis` | `link.go` | `trls link` — add blocked_by link |
| `cmd/trellis` | `decision.go` | `trls decision` — record decision |
| `cmd/trellis` | `reopen.go` | `trls reopen` — reverse transition to open |
| `cmd/trellis` | `merged.go` | `trls merged` — stub for single-branch mode |
| `cmd/trellis` | `render_context.go` | `trls render-context` — context assembly output |
| `cmd/trellis` | `decompose_apply.go` | `trls decompose-apply` — load plan.json |
| `cmd/trellis` | `decompose_revert.go` | `trls decompose-revert` — revert decomposition |
| `cmd/trellis` | `decompose_context.go` | `trls decompose-context` — stub with local filesystem |
| `cmd/trellis` | `validate.go` | `trls validate --ci` — structural validation |
| `internal/ops` | `types.go` | Op type constants, status constants, positional array codec |
| `internal/ops` | `log.go` | Per-worker log file append/read, byte offset tracking |
| `internal/ops` | `parse.go` | JSONL line parsing into typed Op structs |
| `internal/ops` | `schema.go` | SCHEMA file generation |
| `internal/ops` | `ratelimit.go` | Rate limiters (heartbeat 1/min, create 500/commit) |
| `internal/ops` | `ops_test.go` | Unit + property tests for ops package |
| `internal/config` | `config.go` | config.json struct, read/write, project-type detection |
| `internal/config` | `config_test.go` | Tests for config |
| `internal/materialize` | `engine.go` | Incremental replay algorithm, cold-start |
| `internal/materialize` | `checkpoint.go` | checkpoint.json read/write |
| `internal/materialize` | `state.go` | State file writers (index.json, issues/*.json, ready.json) |
| `internal/materialize` | `rollup.go` | Bottom-up story/epic auto-promote |
| `internal/materialize` | `merge_single.go` | Single-branch auto-merge (done→merged) |
| `internal/materialize` | `engine_test.go` | Tests for materialization |
| `internal/ready` | `compute.go` | 4-rule gate, priority sort, ready.json generation |
| `internal/ready` | `ready_test.go` | Tests for ready computation |
| `internal/claim` | `claim.go` | Claim logic, race resolution, TTL, post-claim verify |
| `internal/claim` | `overlap.go` | Scope overlap advisory + auto-note |
| `internal/claim` | `claim_test.go` | Tests for claim system |
| `internal/context` | `assemble.go` | 7-layer context assembly algorithm |
| `internal/context` | `render.go` | Output formatting (agent JSON, human plain text) |
| `internal/context` | `truncate.go` | Priority-based truncation |
| `internal/context` | `context_test.go` | Tests for context assembly |
| `internal/decompose` | `plan.go` | Plan file format v1 parsing/validation |
| `internal/decompose` | `apply.go` | decompose-apply logic (atomicity, idempotency) |
| `internal/decompose` | `revert.go` | decompose-revert (double-entry cancellation) |
| `internal/decompose` | `context_stub.go` | decompose-context stub (local filesystem) |
| `internal/decompose` | `decompose_test.go` | Tests for decomposition |
| `internal/validate` | `validate.go` | Structural validation (cycles, orphans, required fields) |
| `internal/validate` | `validate_test.go` | Tests for validation |
| `internal/hooks` | `runner.go` | Pre-transition hook execution, scope interpolation |
| `internal/hooks` | `hooks_test.go` | Tests for hooks |
| `internal/errors` | `errors.go` | Structured error format (message + state + hint), --debug |
| `internal/worker` | `identity.go` | Worker UUID generation, git config read/write |
| `internal/worker` | `identity_test.go` | Tests for worker identity |

### Modified Files

| File | Changes |
|------|---------|
| `internal/dag/dag.go` | Add `Status`, `Scope`, `Acceptance`, `Provenance` fields to Node; add `AllNodes()`, `Roots()` methods |
| `internal/dag/dag_test.go` | Update tests for new fields |
| `internal/git/git.go` | Add `HasBranchProtection()`, `ConfigGet/Set()`, `InitRepo()` methods |
| `Makefile` | Add `build` target pointing to `cmd/trellis` |
| `go.mod` | Add `cobra`, `uuid` dependencies |

---

## Chunk 1: CLI Foundation + Op Types + Worker Identity (E1-S7 partial, E1-S1 types)

This chunk establishes the CLI entry point, op type system, and worker identity — the foundation everything else builds on.

### Task 1: Create Cobra CLI entry point with `trls version`

**Files:**
- Create: `cmd/trellis/main.go`
- Create: `cmd/trellis/version.go`
- Modify: `Makefile`

- [ ] **Step 1: Write test for version command output**

Create `cmd/trellis/main_test.go`:

```go
package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "trls version")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestVersionCommand -v`
Expected: FAIL — `newRootCmd` not defined

- [ ] **Step 3: Create main.go with root command**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "trls",
		Short: "Trellis — git-native work orchestration",
		SilenceUsage: true,
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")

	root.AddCommand(newVersionCmd())

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create version.go**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print trls version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "trls version %s\n", Version)
			return nil
		},
	}
}
```

- [ ] **Step 5: Add cobra dependency**

Run: `cd /home/brian/development/trellis && go get github.com/spf13/cobra@v1.8.0 && go mod tidy`

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestVersionCommand -v`
Expected: PASS

- [ ] **Step 7: Update Makefile build target**

Add/update the build target:
```makefile
build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/trls ./cmd/trellis
```

- [ ] **Step 8: Verify build**

Run: `cd /home/brian/development/trellis && make build && ./bin/trls version`
Expected: `trls version dev` (or a git SHA)

- [ ] **Step 9: Commit**

```bash
git add cmd/trellis/main.go cmd/trellis/version.go cmd/trellis/main_test.go Makefile go.mod go.sum
git commit -m "feat(cli): add cobra root command and trls version"
```

---

### Task 2: Define Op types and positional array codec

**Files:**
- Create: `internal/ops/types.go`
- Create: `internal/ops/parse.go`
- Create: `internal/ops/ops_test.go`

- [ ] **Step 1: Write test for op parsing round-trip**

Create `internal/ops/ops_test.go`:

```go
package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCreateOp(t *testing.T) {
	line := `["create","task-01",1740700800,"worker-a1",{"title":"Fix auth","parent":"epic-1","type":"task","scope":["src/auth/**"],"acceptance":[]}]`

	op, err := ParseLine([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, OpCreate, op.Type)
	assert.Equal(t, "task-01", op.TargetID)
	assert.Equal(t, int64(1740700800), op.Timestamp)
	assert.Equal(t, "worker-a1", op.WorkerID)
	assert.Equal(t, "Fix auth", op.Payload.Title)
	assert.Equal(t, "epic-1", op.Payload.Parent)
}

func TestParseClaimOp(t *testing.T) {
	line := `["claim","task-01",1740700801,"worker-a1",{"ttl":60}]`

	op, err := ParseLine([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, OpClaim, op.Type)
	assert.Equal(t, 60, op.Payload.TTL)
}

func TestMarshalOp(t *testing.T) {
	op := Op{
		Type:      OpCreate,
		TargetID:  "task-01",
		Timestamp: 1740700800,
		WorkerID:  "worker-a1",
		Payload: Payload{
			Title:  "Fix auth",
			Parent: "epic-1",
			NodeType: "task",
			Scope:  []string{"src/auth/**"},
		},
	}

	line, err := MarshalOp(op)
	require.NoError(t, err)

	// Round-trip
	parsed, err := ParseLine(line)
	require.NoError(t, err)
	assert.Equal(t, op.Type, parsed.Type)
	assert.Equal(t, op.TargetID, parsed.TargetID)
	assert.Equal(t, op.Payload.Title, parsed.Payload.Title)
}

func TestParseInvalidLine(t *testing.T) {
	_, err := ParseLine([]byte(`not json`))
	assert.Error(t, err)

	_, err = ParseLine([]byte(`["unknown","x",0,"w",{}]`))
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create types.go with op type constants and data structures**

```go
package ops

import "encoding/json"

// Op types — all 10 defined in architecture doc section 3.
const (
	OpCreate            = "create"
	OpClaim             = "claim"
	OpHeartbeat         = "heartbeat"
	OpTransition        = "transition"
	OpNote              = "note"
	OpLink              = "link"
	OpSourceLink        = "source-link"
	OpSourceFingerprint = "source-fingerprint"
	OpDAGTransition     = "dag-transition"
	OpDecision          = "decision"
)

// ValidOpTypes for validation.
var ValidOpTypes = map[string]bool{
	OpCreate: true, OpClaim: true, OpHeartbeat: true,
	OpTransition: true, OpNote: true, OpLink: true,
	OpSourceLink: true, OpSourceFingerprint: true,
	OpDAGTransition: true, OpDecision: true,
}

// Issue statuses.
const (
	StatusOpen       = "open"
	StatusClaimed    = "claimed"
	StatusInProgress = "in-progress"
	StatusDone       = "done"
	StatusMerged     = "merged"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
)

// Op represents a single parsed operation from the log.
type Op struct {
	Type      string
	TargetID  string
	Timestamp int64
	WorkerID  string
	Payload   Payload
}

// Payload holds all possible payload fields across op types.
// Only relevant fields are populated for each op type.
type Payload struct {
	// create
	Title            string           `json:"title,omitempty"`
	Parent           string           `json:"parent,omitempty"`
	NodeType         string           `json:"type,omitempty"`
	Scope            []string         `json:"scope,omitempty"`
	Acceptance       json.RawMessage  `json:"acceptance,omitempty"`
	DefinitionOfDone string           `json:"definition_of_done,omitempty"`
	Context          json.RawMessage  `json:"context,omitempty"`
	SourceCitation   json.RawMessage  `json:"source_citation,omitempty"`
	Priority         string           `json:"priority,omitempty"`
	EstComplexity    string           `json:"estimated_complexity,omitempty"`

	// claim
	TTL int `json:"ttl,omitempty"`

	// transition
	To      string `json:"to,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Branch  string `json:"branch,omitempty"`
	PR      string `json:"pr,omitempty"`

	// note
	Msg string `json:"msg,omitempty"`

	// link
	Dep string `json:"dep,omitempty"`
	Rel string `json:"rel,omitempty"`

	// source-link
	SourceID string `json:"source_id,omitempty"`
	Section  string `json:"section,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
	Quote    string `json:"quote,omitempty"`

	// source-fingerprint
	SHA        string `json:"sha,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
	Provider   string `json:"provider,omitempty"`

	// dag-transition
	UncoveredAcknowledged []string `json:"uncovered_acknowledged,omitempty"`

	// decision
	Topic     string   `json:"topic,omitempty"`
	Choice    string   `json:"choice,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Affects   []string `json:"affects,omitempty"`
}
```

- [ ] **Step 4: Create parse.go with positional array parsing**

```go
package ops

import (
	"encoding/json"
	"fmt"
)

// ParseLine parses a single JSONL line in positional array format:
// [op_type, target_id, timestamp, worker_id, payload_object]
func ParseLine(line []byte) (Op, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return Op{}, fmt.Errorf("invalid JSONL line: %w", err)
	}
	if len(raw) < 5 {
		return Op{}, fmt.Errorf("op array must have at least 5 elements, got %d", len(raw))
	}

	var op Op

	if err := json.Unmarshal(raw[0], &op.Type); err != nil {
		return Op{}, fmt.Errorf("invalid op type: %w", err)
	}
	if !ValidOpTypes[op.Type] {
		return Op{}, fmt.Errorf("unknown op type: %s", op.Type)
	}

	if err := json.Unmarshal(raw[1], &op.TargetID); err != nil {
		return Op{}, fmt.Errorf("invalid target_id: %w", err)
	}
	if err := json.Unmarshal(raw[2], &op.Timestamp); err != nil {
		return Op{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	if err := json.Unmarshal(raw[3], &op.WorkerID); err != nil {
		return Op{}, fmt.Errorf("invalid worker_id: %w", err)
	}
	if err := json.Unmarshal(raw[4], &op.Payload); err != nil {
		return Op{}, fmt.Errorf("invalid payload: %w", err)
	}

	return op, nil
}

// MarshalOp serializes an Op to positional array JSONL format.
func MarshalOp(op Op) ([]byte, error) {
	payload, err := json.Marshal(op.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	arr := []interface{}{op.Type, op.TargetID, op.Timestamp, op.WorkerID, json.RawMessage(payload)}
	return json.Marshal(arr)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ops/
git commit -m "feat(ops): add op type constants and positional array codec"
```

---

### Task 3: Add property tests for op parsing

**Files:**
- Modify: `internal/ops/ops_test.go`

- [ ] **Step 1: Add property test for round-trip serialization**

Append to `internal/ops/ops_test.go`:

```go
import (
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestPropOpRoundTrip(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)

	opTypes := gen.OneConstOf(OpCreate, OpClaim, OpHeartbeat, OpTransition,
		OpNote, OpLink, OpSourceLink, OpSourceFingerprint, OpDAGTransition, OpDecision)

	properties.Property("marshal then parse preserves type, target, timestamp, worker", prop.ForAll(
		func(opType string, targetID string, ts int64, workerID string) bool {
			if targetID == "" || workerID == "" {
				return true // skip empty — not valid ops
			}
			op := Op{
				Type:      opType,
				TargetID:  targetID,
				Timestamp: ts,
				WorkerID:  workerID,
			}
			data, err := MarshalOp(op)
			if err != nil {
				return false
			}
			parsed, err := ParseLine(data)
			if err != nil {
				return false
			}
			return parsed.Type == op.Type &&
				parsed.TargetID == op.TargetID &&
				parsed.Timestamp == op.Timestamp &&
				parsed.WorkerID == op.WorkerID
		},
		opTypes,
		gen.AlphaString(),
		gen.Int64(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
```

- [ ] **Step 2: Run to verify it passes**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -run TestProp -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ops/ops_test.go
git commit -m "test(ops): add property test for op round-trip serialization"
```

---

### Task 4: Worker identity — generate UUID, read/write git config

**Files:**
- Create: `internal/worker/identity.go`
- Create: `internal/worker/identity_test.go`

- [ ] **Step 1: Write integration test for worker identity**

Create `internal/worker/identity_test.go`:

```go
package worker

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTempRepo creates a temp git repo for testing.
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", name, args, out)
}

func TestGenerateAndStoreWorkerID(t *testing.T) {
	repo := initTempRepo(t)

	id, err := InitWorker(repo)
	require.NoError(t, err)
	assert.Len(t, id, 36) // UUID format

	// Check reads back
	got, err := GetWorkerID(repo)
	require.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestGetWorkerID_NotSet(t *testing.T) {
	repo := initTempRepo(t)

	_, err := GetWorkerID(repo)
	assert.Error(t, err)
}

func TestCheckWorkerID(t *testing.T) {
	repo := initTempRepo(t)

	ok, _ := CheckWorkerID(repo)
	assert.False(t, ok)

	InitWorker(repo)

	ok, id := CheckWorkerID(repo)
	assert.True(t, ok)
	assert.Len(t, id, 36)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/worker/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create identity.go**

```go
package worker

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

const gitConfigKey = "trellis.worker-id"

// InitWorker generates a new worker UUID and stores it in local git config.
func InitWorker(repoPath string) (string, error) {
	id := uuid.New().String()
	cmd := exec.Command("git", "-C", repoPath, "config", "--local", gitConfigKey, id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to set worker ID: %s: %w", out, err)
	}
	return id, nil
}

// GetWorkerID reads the worker UUID from local git config.
func GetWorkerID(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "config", "--local", gitConfigKey)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("worker ID not configured — run 'trls worker-init': %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckWorkerID returns whether a worker ID is configured, and if so, what it is.
func CheckWorkerID(repoPath string) (bool, string) {
	id, err := GetWorkerID(repoPath)
	if err != nil {
		return false, ""
	}
	return true, id
}
```

- [ ] **Step 4: Add uuid dependency**

Run: `cd /home/brian/development/trellis && go get github.com/google/uuid@v1.6.0 && go mod tidy`

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/worker/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/worker/ go.mod go.sum
git commit -m "feat(worker): add worker identity generation and git config storage"
```

---

### Task 5: `trls worker-init` and `trls worker-init --check` CLI commands

**Files:**
- Create: `cmd/trellis/worker_init.go`
- Modify: `cmd/trellis/main.go` (add command)
- Modify: `cmd/trellis/main_test.go` (add test)

- [ ] **Step 1: Write test for worker-init commands**

Append to `cmd/trellis/main_test.go`:

```go
func TestWorkerInitCommand(t *testing.T) {
	repo := initTempRepo(t)
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"worker-init", "--repo", repo})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Worker ID:")
}

func TestWorkerInitCheckNotConfigured(t *testing.T) {
	repo := initTempRepo(t)
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"worker-init", "--check", "--repo", repo})

	err := cmd.Execute()
	assert.Error(t, err) // should fail — no worker ID
}

func TestWorkerInitCheckConfigured(t *testing.T) {
	repo := initTempRepo(t)

	// First init
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"worker-init", "--repo", repo})
	cmd1.Execute()

	// Then check
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"worker-init", "--check", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Worker ID:")
}
```

Also add the `initTempRepo` and `run` helpers to the test file:

```go
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", name, args, out)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestWorkerInit -v`
Expected: FAIL — command not registered

- [ ] **Step 3: Create worker_init.go**

```go
package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newWorkerInitCmd() *cobra.Command {
	var check bool
	var repoPath string

	cmd := &cobra.Command{
		Use:   "worker-init",
		Short: "Generate or check worker identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}

			if check {
				ok, id := worker.CheckWorkerID(repoPath)
				if !ok {
					return fmt.Errorf("no worker ID configured — run 'trls worker-init'")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", id)
				return nil
			}

			id, err := worker.InitWorker(repoPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Worker ID: %s\n", id)
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "verify existing worker ID without modifying state")
	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")

	return cmd
}
```

- [ ] **Step 4: Register command in main.go**

Add `root.AddCommand(newWorkerInitCmd())` in `newRootCmd()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/worker_init.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls worker-init and worker-init --check commands"
```

---

### Task 6: Op log file append and read

**Files:**
- Create: `internal/ops/log.go`
- Modify: `internal/ops/ops_test.go`

- [ ] **Step 1: Write test for log append and read**

Add to `internal/ops/ops_test.go`:

```go
func TestLogAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Test task", NodeType: "task"}}
	op2 := Op{Type: OpClaim, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a1",
		Payload: Payload{TTL: 60}}

	require.NoError(t, AppendOp(logPath, op1))
	require.NoError(t, AppendOp(logPath, op2))

	ops, err := ReadLog(logPath)
	require.NoError(t, err)
	assert.Len(t, ops, 2)
	assert.Equal(t, OpCreate, ops[0].Type)
	assert.Equal(t, OpClaim, ops[1].Type)
}

func TestReadLogFromOffset(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	// Get current offset
	info, _ := os.Stat(logPath)
	offset := info.Size()

	op2 := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "Second"}}
	require.NoError(t, AppendOp(logPath, op2))

	ops, err := ReadLogFromOffset(logPath, offset)
	require.NoError(t, err)
	assert.Len(t, ops, 1)
	assert.Equal(t, "Second", ops[0].Payload.Msg)
}

func TestValidateWorkerIDInLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Op with wrong worker ID
	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b2",
		Payload: Payload{Title: "Bad", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op))

	ops, err := ReadLogValidated(logPath, "worker-a1")
	require.NoError(t, err)
	assert.Len(t, ops, 0) // rejected — worker ID mismatch
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -run TestLog -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Create log.go**

```go
package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppendOp appends a single op to the log file as a JSONL line.
func AppendOp(logPath string, op Op) error {
	return AppendOps(logPath, []Op{op})
}

// AppendOps appends multiple ops atomically in a single file write.
func AppendOps(logPath string, ops []Op) error {
	var buf []byte
	for _, op := range ops {
		line, err := MarshalOp(op)
		if err != nil {
			return err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("write to log %s: %w", logPath, err)
	}
	return nil
}

// ReadLog reads all ops from a log file.
func ReadLog(logPath string) ([]Op, error) {
	return ReadLogFromOffset(logPath, 0)
}

// ReadLogFromOffset reads ops starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) ([]Op, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	var ops []Op
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		op, err := ParseLine(line)
		if err != nil {
			// Skip corrupt lines per spec — log warning
			continue
		}
		ops = append(ops, op)
	}
	return ops, scanner.Err()
}

// ReadLogValidated reads ops and filters out those with mismatched worker IDs.
// The expected worker ID is extracted from the log filename: <worker-id>.log
func ReadLogValidated(logPath string, expectedWorkerID string) ([]Op, error) {
	all, err := ReadLog(logPath)
	if err != nil {
		return nil, err
	}
	var valid []Op
	for _, op := range all {
		if op.WorkerID == expectedWorkerID {
			valid = append(valid, op)
		}
	}
	return valid, nil
}

// WorkerIDFromFilename extracts the worker ID from a log filename.
// "worker-a1.log" -> "worker-a1"
func WorkerIDFromFilename(logPath string) string {
	base := filepath.Base(logPath)
	return strings.TrimSuffix(base, ".log")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -run TestLog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ops/log.go internal/ops/ops_test.go
git commit -m "feat(ops): add log file append, read, offset-based read, and worker ID validation"
```

---

### Task 7: SCHEMA file generation and rate limiters

**Files:**
- Create: `internal/ops/schema.go`
- Create: `internal/ops/ratelimit.go`
- Modify: `internal/ops/ops_test.go`

- [ ] **Step 1: Write test for SCHEMA generation**

```go
func TestGenerateSchema(t *testing.T) {
	schema := GenerateSchema()
	assert.Contains(t, schema, "op_type")
	assert.Contains(t, schema, "target_id")
	assert.Contains(t, schema, "timestamp")
	assert.Contains(t, schema, "worker_id")
	assert.Contains(t, schema, "payload")
}
```

- [ ] **Step 2: Write test for rate limiter**

```go
func TestHeartbeatRateLimiter(t *testing.T) {
	rl := NewRateLimiter()

	// First heartbeat should be allowed
	assert.True(t, rl.AllowHeartbeat("task-01", 1000))

	// Heartbeat within 60 seconds should be rejected
	assert.False(t, rl.AllowHeartbeat("task-01", 1030))

	// Heartbeat after 60 seconds should be allowed
	assert.True(t, rl.AllowHeartbeat("task-01", 1061))

	// Different task should be independent
	assert.True(t, rl.AllowHeartbeat("task-02", 1030))
}

func TestCreateRateLimiter(t *testing.T) {
	rl := NewRateLimiter()

	for i := 0; i < 500; i++ {
		assert.True(t, rl.AllowCreate())
	}
	assert.False(t, rl.AllowCreate()) // 501st should fail

	rl.ResetCreateCount() // simulate commit boundary
	assert.True(t, rl.AllowCreate())
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -run "TestGenerate|TestHeartbeat|TestCreate" -v`
Expected: FAIL

- [ ] **Step 4: Create schema.go**

```go
package ops

// GenerateSchema returns the SCHEMA file content defining positional array format.
func GenerateSchema() string {
	return `# Trellis Op Log Schema v1
#
# Each line is a JSON array: [op_type, target_id, timestamp, worker_id, payload]
#
# Position 0: op_type (string) — one of: create, claim, heartbeat, transition,
#             note, link, source-link, source-fingerprint, dag-transition, decision
# Position 1: target_id (string) — issue/node/source ID this op targets
# Position 2: timestamp (integer) — Unix epoch seconds
# Position 3: worker_id (string) — UUID of the worker emitting this op
# Position 4: payload (object) — op-type-specific fields (see below)
#
# Forward compatibility: new fields may be appended to the array.
# Readers MUST ignore extra positions. Missing positions get defaults.
#
# Payload fields by op type:
#   create:             title, parent, type, scope, acceptance, definition_of_done,
#                       context, source_citation, priority, estimated_complexity
#   claim:              ttl
#   heartbeat:          (empty object)
#   transition:         to, outcome, branch (optional), pr (optional)
#   note:               msg
#   link:               dep, rel
#   source-link:        source_id, section, anchor, quote
#   source-fingerprint: sha, version_id, provider
#   dag-transition:     to, uncovered_acknowledged
#   decision:           topic, choice, rationale, affects
`
}
```

- [ ] **Step 5: Create ratelimit.go**

```go
package ops

import "sync"

// RateLimiter enforces CLI-side rate limits per the spec:
// - Heartbeats: max 1 per minute per issue
// - Creates: max 500 per commit batch
type RateLimiter struct {
	mu              sync.Mutex
	lastHeartbeat   map[string]int64 // issue ID -> last heartbeat timestamp
	createCount     int
	createLimit     int
	heartbeatMinGap int64 // seconds
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		lastHeartbeat:   make(map[string]int64),
		createLimit:     500,
		heartbeatMinGap: 60,
	}
}

func (r *RateLimiter) AllowHeartbeat(issueID string, nowEpoch int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	last, ok := r.lastHeartbeat[issueID]
	if ok && (nowEpoch-last) < r.heartbeatMinGap {
		return false
	}
	r.lastHeartbeat[issueID] = nowEpoch
	return true
}

func (r *RateLimiter) AllowCreate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.createCount >= r.createLimit {
		return false
	}
	r.createCount++
	return true
}

func (r *RateLimiter) ResetCreateCount() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCount = 0
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/ops/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ops/schema.go internal/ops/ratelimit.go internal/ops/ops_test.go
git commit -m "feat(ops): add SCHEMA file generation and rate limiters"
```

---

## Chunk 2: Single-Branch Init + Config (E1-S7 complete)

### Task 8: Config struct and project-type detection

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write test for config round-trip and project detection**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		Mode:            "single-branch",
		ProjectType:     "go",
		DefaultTTL:      60,
		TokenBudget:     1600,
		Hooks:           []HookConfig{},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", loaded.Mode)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, 60, loaded.DefaultTTL)
}

func TestDetectProjectType(t *testing.T) {
	dir := t.TempDir()

	// No marker files — unknown
	assert.Equal(t, "unknown", DetectProjectType(dir))

	// Add go.mod
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	assert.Equal(t, "go", DetectProjectType(dir))
}

func TestDetectProjectTypePriority(t *testing.T) {
	dir := t.TempDir()

	// Both go.mod and package.json — go wins (alphabetical file check order doesn't matter, we check go.mod first)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	assert.Equal(t, "go", DetectProjectType(dir))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/config/ -v`
Expected: FAIL

- [ ] **Step 3: Create config.go**

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Mode        string       `json:"mode"`         // "single-branch" or "dual-branch"
	ProjectType string       `json:"project_type"`
	DefaultTTL  int          `json:"default_ttl"`   // minutes
	TokenBudget int          `json:"token_budget"`
	Hooks       []HookConfig `json:"hooks"`
}

type HookConfig struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Required bool   `json:"required"`
}

func WriteConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// DetectProjectType checks for known project marker files.
func DetectProjectType(repoPath string) string {
	markers := []struct {
		file     string
		projType string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"Cargo.toml", "rust"},
		{"Makefile", "make"},
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(repoPath, m.file)); err == nil {
			return m.projType
		}
	}
	return "unknown"
}

// DefaultConfig returns a config with sensible defaults for single-branch mode.
func DefaultConfig(projectType string) Config {
	return Config{
		Mode:        "single-branch",
		ProjectType: projectType,
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add config.json struct, read/write, and project type detection"
```

---

### Task 9: `trls init` for single-branch mode

**Files:**
- Create: `cmd/trellis/init.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write test for init command**

Add to `cmd/trellis/main_test.go`:

```go
func TestInitCommand_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"init", "--repo", repo})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "single-branch")

	// Verify .issues directory structure was created
	assert.DirExists(t, filepath.Join(repo, ".issues"))
	assert.DirExists(t, filepath.Join(repo, ".issues", "ops"))
	assert.DirExists(t, filepath.Join(repo, ".issues", "state"))
	assert.FileExists(t, filepath.Join(repo, ".issues", "config.json"))
	assert.FileExists(t, filepath.Join(repo, ".issues", "ops", "SCHEMA"))
}

func TestInitCommand_Idempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init twice should not error
	for i := 0; i < 2; i++ {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"init", "--repo", repo})
		assert.NoError(t, cmd.Execute())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestInitCommand -v`
Expected: FAIL

- [ ] **Step 3: Create init.go**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Trellis in the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			return runInit(cmd, repoPath)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	return cmd
}

func runInit(cmd *cobra.Command, repoPath string) error {
	issuesDir := filepath.Join(repoPath, ".issues")

	// Create directory structure
	dirs := []string{
		filepath.Join(issuesDir, "ops"),
		filepath.Join(issuesDir, "state"),
		filepath.Join(issuesDir, "state", "issues"),
		filepath.Join(issuesDir, "templates"),
		filepath.Join(issuesDir, "hooks"),
		filepath.Join(issuesDir, "review"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0644); err != nil {
		return fmt.Errorf("write SCHEMA: %w", err)
	}

	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return fmt.Errorf("init worker: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized Trellis in single-branch mode at %s\n", issuesDir)
	return nil
}
```

- [ ] **Step 4: Register init command in main.go**

Add `root.AddCommand(newInitCmd())` in `newRootCmd()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestInitCommand -v`
Expected: PASS

- [ ] **Step 6: Run all tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/trellis/init.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls init for single-branch mode"
```

---

## Chunk 3: Materialization Engine (E1-S2)

### Task 10: Checkpoint read/write

**Files:**
- Create: `internal/materialize/checkpoint.go`
- Create: `internal/materialize/checkpoint_test.go`

- [ ] **Step 1: Write test for checkpoint round-trip**

Create `internal/materialize/checkpoint_test.go`:

```go
package materialize

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := Checkpoint{
		LastCommitSHA: "abc123",
		ByteOffsets:   map[string]int64{"worker-a1.log": 1024, "worker-b2.log": 512},
	}

	require.NoError(t, WriteCheckpoint(path, cp))

	loaded, err := LoadCheckpoint(path)
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.LastCommitSHA)
	assert.Equal(t, int64(1024), loaded.ByteOffsets["worker-a1.log"])
}

func TestLoadCheckpoint_Missing(t *testing.T) {
	cp, err := LoadCheckpoint("/nonexistent/checkpoint.json")
	require.NoError(t, err) // missing checkpoint = fresh start
	assert.Equal(t, "", cp.LastCommitSHA)
	assert.NotNil(t, cp.ByteOffsets)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -v`
Expected: FAIL

- [ ] **Step 3: Create checkpoint.go**

```go
package materialize

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Checkpoint struct {
	LastCommitSHA string           `json:"last_materialized_commit"`
	ByteOffsets   map[string]int64 `json:"byte_offsets"`
}

func WriteCheckpoint(path string, cp Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadCheckpoint(path string) (Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Checkpoint{ByteOffsets: make(map[string]int64)}, nil
		}
		return Checkpoint{}, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, fmt.Errorf("parse checkpoint: %w", err)
	}
	if cp.ByteOffsets == nil {
		cp.ByteOffsets = make(map[string]int64)
	}
	return cp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/
git commit -m "feat(materialize): add checkpoint read/write"
```

---

### Task 11: State file types — materialized issue, index, ready.json

**Files:**
- Create: `internal/materialize/state.go`
- Create: `internal/materialize/state_test.go`

- [ ] **Step 1: Write test for state file round-trips**

Create `internal/materialize/state_test.go`:

```go
package materialize

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:       "task-01",
		Type:     "task",
		Status:   "open",
		Title:    "Fix auth",
		Parent:   "story-01",
		Scope:    []string{"src/auth/**"},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", loaded.ID)
	assert.Equal(t, "Fix auth", loaded.Title)
}

func TestIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")

	index := Index{
		"task-01": IndexEntry{Status: "open", Type: "task", Title: "Fix auth", Parent: "story-01"},
	}

	require.NoError(t, WriteIndex(indexPath, index))

	loaded, err := LoadIndex(indexPath)
	require.NoError(t, err)
	assert.Equal(t, "open", loaded["task-01"].Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -run "TestIssue|TestIndex" -v`
Expected: FAIL

- [ ] **Step 3: Create state.go**

```go
package materialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Issue represents the full materialized state of a single work item.
type Issue struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Status           string          `json:"status"`
	Title            string          `json:"title"`
	Parent           string          `json:"parent,omitempty"`
	Children         []string        `json:"children"`
	BlockedBy        []string        `json:"blocked_by"`
	Blocks           []string        `json:"blocks"`
	Assignee         string          `json:"assignee,omitempty"`
	Priority         string          `json:"priority,omitempty"`
	EstComplexity    string          `json:"estimated_complexity,omitempty"`
	DefinitionOfDone string          `json:"definition_of_done,omitempty"`
	Scope            []string        `json:"scope"`
	ContextFiles     []string        `json:"context_files,omitempty"`
	Acceptance       json.RawMessage `json:"acceptance,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	SourceCitation   json.RawMessage `json:"source_citation,omitempty"`
	Provenance       Provenance      `json:"provenance"`
	DecisionRefs     []string        `json:"decision_refs"`
	Outcome          string          `json:"outcome,omitempty"`
	PriorOutcomes    []string        `json:"prior_outcomes,omitempty"`
	Notes            []Note          `json:"notes,omitempty"`
	Decisions        []Decision      `json:"decisions,omitempty"`
	ClaimedBy        string          `json:"claimed_by,omitempty"`
	ClaimedAt        int64           `json:"claimed_at,omitempty"`
	ClaimTTL         int             `json:"claim_ttl,omitempty"`
	LastHeartbeat    int64           `json:"last_heartbeat,omitempty"`
	Branch           string          `json:"branch,omitempty"`
	PR               string          `json:"pr,omitempty"`
	Updated          int64           `json:"updated"`
}

type Provenance struct {
	Method       string `json:"method"`
	Confidence   string `json:"confidence"`
	SourceWorker string `json:"source_worker"`
}

type Note struct {
	WorkerID  string `json:"worker_id"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
}

type Decision struct {
	Topic     string   `json:"topic"`
	Choice    string   `json:"choice"`
	Rationale string   `json:"rationale"`
	Affects   []string `json:"affects"`
	WorkerID  string   `json:"worker_id"`
	Timestamp int64    `json:"timestamp"`
}

// IndexEntry is the denormalized summary stored in index.json.
type IndexEntry struct {
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	Parent    string   `json:"parent,omitempty"`
	Children  []string `json:"children,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	Blocks    []string `json:"blocks,omitempty"`
	Assignee  string   `json:"assignee,omitempty"`
	Updated   int64    `json:"updated"`
	Title     string   `json:"title"`
	Outcome   string   `json:"outcome,omitempty"`
	Scope     []string `json:"scope,omitempty"`
}

type Index map[string]IndexEntry

func WriteIssue(issuesDir string, issue Issue) error {
	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	path := filepath.Join(issuesDir, issue.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func LoadIssue(path string) (Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	return issue, json.Unmarshal(data, &issue)
}

func WriteIndex(path string, index Index) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadIndex(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Index), nil
		}
		return nil, err
	}
	var index Index
	return index, json.Unmarshal(data, &index)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/state.go internal/materialize/state_test.go
git commit -m "feat(materialize): add Issue, Index, and state file read/write"
```

---

### Task 12: Materialization engine — apply ops to state

**Files:**
- Create: `internal/materialize/engine.go`
- Create: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write test for applying ops to build state**

Create `internal/materialize/engine_test.go`:

```go
package materialize

import (
	"testing"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCreateOp(t *testing.T) {
	state := NewState()

	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Fix auth", Parent: "story-01", NodeType: "task",
			Scope: []string{"src/auth/**"}, DefinitionOfDone: "Tests pass"},
	}

	require.NoError(t, state.ApplyOp(op))

	issue := state.Issues["task-01"]
	assert.Equal(t, "task-01", issue.ID)
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "Fix auth", issue.Title)
	assert.Equal(t, "story-01", issue.Parent)
}

func TestApplyClaimOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})

	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})

	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "w1", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
}

func TestApplyTransitionOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
	state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Fixed it"}})

	issue := state.Issues["task-01"]
	assert.Equal(t, "done", issue.Status)
	assert.Equal(t, "Fixed it", issue.Outcome)
}

func TestApplyNoteOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "Found edge case"}})

	assert.Len(t, state.Issues["task-01"].Notes, 1)
	assert.Equal(t, "Found edge case", state.Issues["task-01"].Notes[0].Msg)
}

func TestApplyLinkOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}})

	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.Contains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyDecisionOp_LastWriteWins(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Topic: "db", Choice: "postgres", Rationale: "mature"}})
	state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Topic: "db", Choice: "sqlite", Rationale: "simpler"}})

	// Last write wins for same topic
	decisions := state.Issues["task-01"].Decisions
	active := activeDecisionForTopic(decisions, "db")
	assert.Equal(t, "sqlite", active.Choice)
}

func TestSingleBranchAutoMerge(t *testing.T) {
	state := NewState()
	state.SingleBranchMode = true

	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
	state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Done"}})

	// In single-branch mode, done immediately becomes merged
	assert.Equal(t, "merged", state.Issues["task-01"].Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -run "TestApply|TestSingleBranch" -v`
Expected: FAIL

- [ ] **Step 3: Create engine.go**

```go
package materialize

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
)

// State holds the complete materialized state built from op replay.
type State struct {
	Issues           map[string]*Issue
	SingleBranchMode bool
}

func NewState() *State {
	return &State{
		Issues: make(map[string]*Issue),
	}
}

// ApplyOp applies a single op to the materialized state.
func (s *State) ApplyOp(op ops.Op) error {
	switch op.Type {
	case ops.OpCreate:
		return s.applyCreate(op)
	case ops.OpClaim:
		return s.applyClaim(op)
	case ops.OpHeartbeat:
		return s.applyHeartbeat(op)
	case ops.OpTransition:
		return s.applyTransition(op)
	case ops.OpNote:
		return s.applyNote(op)
	case ops.OpLink:
		return s.applyLink(op)
	case ops.OpDecision:
		return s.applyDecision(op)
	case ops.OpSourceLink, ops.OpSourceFingerprint, ops.OpDAGTransition:
		// Handled but no state impact in E1
		return nil
	default:
		return fmt.Errorf("unknown op type: %s", op.Type)
	}
}

func (s *State) applyCreate(op ops.Op) error {
	if _, exists := s.Issues[op.TargetID]; exists {
		return nil // idempotent — skip duplicate creates
	}

	issue := &Issue{
		ID:               op.TargetID,
		Type:             op.Payload.NodeType,
		Status:           ops.StatusOpen,
		Title:            op.Payload.Title,
		Parent:           op.Payload.Parent,
		Scope:            op.Payload.Scope,
		Priority:         op.Payload.Priority,
		EstComplexity:    op.Payload.EstComplexity,
		DefinitionOfDone: op.Payload.DefinitionOfDone,
		Acceptance:       op.Payload.Acceptance,
		Context:          op.Payload.Context,
		SourceCitation:   op.Payload.SourceCitation,
		Provenance: Provenance{
			Method:       "decomposed",
			Confidence:   "verified",
			SourceWorker: op.WorkerID,
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
		Updated:      op.Timestamp,
	}

	s.Issues[op.TargetID] = issue

	// Register as child of parent
	if op.Payload.Parent != "" {
		if parent, ok := s.Issues[op.Payload.Parent]; ok {
			parent.Children = appendUnique(parent.Children, op.TargetID)
		}
	}

	return nil
}

func (s *State) applyClaim(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("claim: issue %s not found", op.TargetID)
	}
	issue.Status = ops.StatusClaimed
	issue.ClaimedBy = op.WorkerID
	issue.ClaimedAt = op.Timestamp
	issue.ClaimTTL = op.Payload.TTL
	issue.LastHeartbeat = op.Timestamp
	issue.Updated = op.Timestamp

	// Set parent to in-progress if not already
	s.promoteParentToInProgress(issue.Parent)
	return nil
}

func (s *State) applyHeartbeat(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil // ignore heartbeat for unknown issue
	}
	issue.LastHeartbeat = op.Timestamp
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyTransition(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("transition: issue %s not found", op.TargetID)
	}

	newStatus := op.Payload.To

	// Handle reverse transitions
	if newStatus == ops.StatusOpen && issue.Status == ops.StatusDone {
		// Reopen: preserve outcome in prior_outcomes
		if issue.Outcome != "" {
			issue.PriorOutcomes = append(issue.PriorOutcomes, issue.Outcome)
			issue.Outcome = ""
		}
		issue.ClaimedBy = ""
		issue.ClaimedAt = 0
	}

	issue.Status = newStatus
	issue.Updated = op.Timestamp

	if op.Payload.Outcome != "" {
		issue.Outcome = op.Payload.Outcome
	}
	if op.Payload.Branch != "" {
		issue.Branch = op.Payload.Branch
	}
	if op.Payload.PR != "" {
		issue.PR = op.Payload.PR
	}

	// Single-branch mode: done → merged immediately
	if s.SingleBranchMode && newStatus == ops.StatusDone {
		issue.Status = ops.StatusMerged
	}

	return nil
}

func (s *State) applyNote(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil // ignore notes for unknown issues
	}
	issue.Notes = append(issue.Notes, Note{
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
		Msg:       op.Payload.Msg,
	})
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyLink(op ops.Op) error {
	source, ok := s.Issues[op.TargetID]
	if !ok {
		return fmt.Errorf("link: source issue %s not found", op.TargetID)
	}

	if op.Payload.Rel == "blocked_by" {
		source.BlockedBy = appendUnique(source.BlockedBy, op.Payload.Dep)
		if dep, ok := s.Issues[op.Payload.Dep]; ok {
			dep.Blocks = appendUnique(dep.Blocks, op.TargetID)
		}
	}
	source.Updated = op.Timestamp
	return nil
}

func (s *State) applyDecision(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.Decisions = append(issue.Decisions, Decision{
		Topic:     op.Payload.Topic,
		Choice:    op.Payload.Choice,
		Rationale: op.Payload.Rationale,
		Affects:   op.Payload.Affects,
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
	})
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) promoteParentToInProgress(parentID string) {
	if parentID == "" {
		return
	}
	parent, ok := s.Issues[parentID]
	if !ok {
		return
	}
	if parent.Status == ops.StatusOpen {
		parent.Status = ops.StatusInProgress
	}
}

// RunRollup promotes stories/epics to done/merged when all children are merged.
func (s *State) RunRollup() {
	// Bottom-up: process by depth (deepest first).
	// Simple approach: iterate until no changes.
	changed := true
	for changed {
		changed = false
		for _, issue := range s.Issues {
			if issue.Type == "task" {
				continue
			}
			if issue.Status == ops.StatusMerged || issue.Status == ops.StatusCancelled {
				continue
			}
			if len(issue.Children) == 0 {
				continue
			}
			allMerged := true
			for _, childID := range issue.Children {
				child, ok := s.Issues[childID]
				if !ok || child.Status != ops.StatusMerged {
					allMerged = false
					break
				}
			}
			if allMerged && issue.Status != ops.StatusMerged {
				issue.Status = ops.StatusMerged
				changed = true
			}
		}
	}
}

// BuildIndex creates the denormalized index from current state.
func (s *State) BuildIndex() Index {
	index := make(Index, len(s.Issues))
	for id, issue := range s.Issues {
		index[id] = IndexEntry{
			Status:    issue.Status,
			Type:      issue.Type,
			Parent:    issue.Parent,
			Children:  issue.Children,
			BlockedBy: issue.BlockedBy,
			Blocks:    issue.Blocks,
			Assignee:  issue.ClaimedBy,
			Updated:   issue.Updated,
			Title:     issue.Title,
			Outcome:   issue.Outcome,
			Scope:     issue.Scope,
		}
	}
	return index
}

// activeDecisionForTopic returns the latest decision for a given topic.
func activeDecisionForTopic(decisions []Decision, topic string) Decision {
	var latest Decision
	for _, d := range decisions {
		if d.Topic == topic && d.Timestamp > latest.Timestamp {
			latest = d
		}
	}
	return latest
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat(materialize): add op replay engine with all op types and single-branch auto-merge"
```

---

### Task 13: Full materialization pipeline — read logs, apply, write state files

**Files:**
- Create: `internal/materialize/pipeline.go`
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write integration test for full pipeline**

Add to `internal/materialize/engine_test.go`:

```go
func TestMaterializePipeline(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	os.MkdirAll(opsDir, 0755)
	os.MkdirAll(issuesDir, 0755)

	// Write some ops to a log file
	logPath := filepath.Join(opsDir, "worker-a1.log")
	ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}})
	ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "epic-01"}})
	ops.AppendOp(logPath, ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a1", Payload: ops.Payload{TTL: 60}})

	result, err := Materialize(dir, true) // single-branch mode
	require.NoError(t, err)
	assert.Equal(t, 2, result.IssueCount)

	// Verify state files were written
	assert.FileExists(t, filepath.Join(stateDir, "index.json"))
	assert.FileExists(t, filepath.Join(issuesDir, "task-01.json"))
	assert.FileExists(t, filepath.Join(stateDir, "checkpoint.json"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -run TestMaterializePipeline -v`
Expected: FAIL

- [ ] **Step 3: Create pipeline.go**

```go
package materialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/ops"
)

type Result struct {
	IssueCount   int
	OpsProcessed int
	FullReplay   bool
}

// Materialize runs the full materialization pipeline:
// 1. Load checkpoint
// 2. Read new ops from all log files (from byte offsets)
// 3. Apply ops to state
// 4. Run rollup
// 5. Write state files
// 6. Update checkpoint
func Materialize(issuesDir string, singleBranch bool) (Result, error) {
	opsDir := filepath.Join(issuesDir, "ops")
	stateDir := filepath.Join(issuesDir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")

	// Ensure state directories exist
	os.MkdirAll(issuesStateDir, 0755)

	// Load checkpoint
	cp, err := LoadCheckpoint(checkpointPath)
	if err != nil {
		return Result{}, fmt.Errorf("load checkpoint: %w", err)
	}

	// Find all log files
	entries, err := os.ReadDir(opsDir)
	if err != nil {
		return Result{}, fmt.Errorf("read ops dir: %w", err)
	}

	// Collect all ops from all log files
	var allOps []ops.Op
	newOffsets := make(map[string]int64)
	fullReplay := len(cp.ByteOffsets) == 0

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
		workerID := ops.WorkerIDFromFilename(logPath)

		offset := cp.ByteOffsets[entry.Name()]
		logOps, err := ops.ReadLogFromOffset(logPath, offset)
		if err != nil {
			return Result{}, fmt.Errorf("read log %s: %w", entry.Name(), err)
		}

		// Validate worker IDs
		for _, op := range logOps {
			if op.WorkerID != workerID {
				continue // skip mismatched worker IDs
			}
			allOps = append(allOps, op)
		}

		// Update byte offset
		info, _ := os.Stat(logPath)
		if info != nil {
			newOffsets[entry.Name()] = info.Size()
		}
	}

	// Sort all ops by timestamp for deterministic replay
	sortOpsByTimestamp(allOps)

	// If full replay, start fresh; otherwise load existing state
	state := NewState()
	state.SingleBranchMode = singleBranch

	// E1: Always full replay from ops. Byte offsets already limit which ops we
	// read from each log file, so this is O(new ops) per invocation.
	// Full incremental state serialization deferred until performance requires it.
	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			// Log warning but continue — corrupt ops are skipped
			continue
		}
	}

	// Run rollup
	state.RunRollup()

	// Write state files
	index := state.BuildIndex()
	if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
		return Result{}, fmt.Errorf("write index: %w", err)
	}

	for _, issue := range state.Issues {
		if err := WriteIssue(issuesStateDir, *issue); err != nil {
			return Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
		}
	}

	// Write ready.json placeholder (recomputed on-demand by `trls ready`)
	readyPath := filepath.Join(stateDir, "ready.json")
	os.WriteFile(readyPath, []byte("[]"), 0644)

	// Cold-start progress indicator
	if fullReplay && len(allOps) > 100 {
		fmt.Fprintf(os.Stderr, "Full replay: processed %d ops across %d issues\n", len(allOps), len(state.Issues))
	}

	// Write checkpoint
	newCp := Checkpoint{
		ByteOffsets: newOffsets,
	}
	if err := WriteCheckpoint(checkpointPath, newCp); err != nil {
		return Result{}, fmt.Errorf("write checkpoint: %w", err)
	}

	return Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
		FullReplay:   fullReplay,
	}, nil
}

// sortOpsByTimestamp sorts ops by timestamp, stable.
func sortOpsByTimestamp(allOps []ops.Op) {
	// Simple insertion sort — op counts are small in practice
	for i := 1; i < len(allOps); i++ {
		for j := i; j > 0 && allOps[j].Timestamp < allOps[j-1].Timestamp; j-- {
			allOps[j], allOps[j-1] = allOps[j-1], allOps[j]
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/pipeline.go internal/materialize/engine_test.go
git commit -m "feat(materialize): add full materialization pipeline with log reading and state file output"
```

---

### Task 14: `trls materialize` CLI command

**Files:**
- Create: `cmd/trellis/materialize.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write test for materialize command**

Add to `cmd/trellis/main_test.go`:

```go
func TestMaterializeCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init trellis first
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd1.Execute())

	// Run materialize (empty — should succeed with 0 issues)
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"materialize", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Create materialize.go**

```go
package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newMaterializeCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Replay op logs and update materialized state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			result, err := materialize.Materialize(issuesDir, true) // E1: always single-branch
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Materialized %d issues from %d ops", result.IssueCount, result.OpsProcessed)
			if result.FullReplay {
				fmt.Fprint(cmd.OutOrStdout(), " (full replay)")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	return cmd
}
```

- [ ] **Step 3: Register in main.go and run tests**

Add `root.AddCommand(newMaterializeCmd())` in `newRootCmd()`.

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestMaterialize -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/materialize.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls materialize command"
```

---

### Task 15: Property tests for materialization invariants

**Files:**
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Add property test — random op sequences never crash**

```go
func TestPropRandomOpsNeverCrash(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 500

	properties := gopter.NewProperties(params)

	opTypeGen := gen.OneConstOf(ops.OpCreate, ops.OpClaim, ops.OpHeartbeat,
		ops.OpTransition, ops.OpNote, ops.OpLink, ops.OpDecision)

	properties.Property("random op sequences never panic", prop.ForAll(
		func(opType string, targetID string, ts int64) bool {
			if targetID == "" {
				return true
			}
			state := NewState()
			state.SingleBranchMode = true

			// Always create the target first
			state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})

			// Apply random op — should not panic
			state.ApplyOp(ops.Op{Type: opType, TargetID: targetID, Timestamp: ts + 1,
				WorkerID: "w1", Payload: ops.Payload{TTL: 60, To: "done", Msg: "test",
					Dep: "other", Rel: "blocked_by", Topic: "t", Choice: "c"}})

			return true // if we get here without panic, pass
		},
		opTypeGen,
		gen.AlphaString(),
		gen.Int64Range(0, 1<<50),
	))

	properties.TestingRun(t)
}

func TestPropCreateIdempotent(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	properties.Property("duplicate creates are idempotent", prop.ForAll(
		func(id string) bool {
			if id == "" {
				return true
			}
			state := NewState()
			op := ops.Op{Type: ops.OpCreate, TargetID: id, Timestamp: 100,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}

			state.ApplyOp(op)
			state.ApplyOp(op) // duplicate

			return len(state.Issues) == 1 && state.Issues[id].Title == "T"
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
```

- [ ] **Step 2: Run property tests**

Run: `cd /home/brian/development/trellis && go test ./internal/materialize/ -run TestProp -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/materialize/engine_test.go
git commit -m "test(materialize): add property tests for op replay invariants"
```

---

## Chunk 4: Ready Task Computation + Core Status Ops (E1-S3, E1-S6)

### Task 16: Ready task computation — 4-rule gate + priority sort

**Files:**
- Create: `internal/ready/compute.go`
- Create: `internal/ready/ready_test.go`

- [ ] **Step 1: Write tests for ready task rules**

Create `internal/ready/ready_test.go`:

```go
package ready

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
)

func TestReadyTask_AllRulesMet(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story", Children: []string{"task-01"}},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 1)
	assert.Equal(t, "task-01", ready[0].Issue)
}

func TestReadyTask_BlockerNotMerged(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "done", Type: "task"}, // done but not merged
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 0) // blocked — blocker must be merged, not just done
}

func TestReadyTask_BlockerMerged(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "merged", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 1)
}

func TestReadyTask_ParentNotInProgress(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "open", Type: "story"}, // parent is open, not in-progress
		"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 0)
}

func TestReadyTask_NoParent(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task"},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 1) // no parent = ready
}

func TestReadyTask_InferredRequiresConfirmation(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task",
			Provenance: materialize.Provenance{Confidence: "inferred"}},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 1)
	assert.True(t, ready[0].RequiresConfirmation)
}

func TestReadyTask_PrioritySort(t *testing.T) {
	index := materialize.Index{
		"task-a": {Status: "open", Type: "task", Blocks: []string{"task-c", "task-d"}},
		"task-b": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: "open", Type: "task", Priority: "medium", Blocks: []string{"task-c", "task-d"}},
		"task-b": {ID: "task-b", Status: "open", Type: "task", Priority: "high"},
	}

	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 2)
	// high priority sorts first
	assert.Equal(t, "task-b", ready[0].Issue)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/ready/ -v`
Expected: FAIL

- [ ] **Step 3: Create compute.go**

```go
package ready

import (
	"sort"
	"time"

	claimPkg "github.com/scullxbones/trellis/internal/claim"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
)

// ReadyEntry represents a task in the ready queue.
type ReadyEntry struct {
	Issue                string   `json:"issue"`
	Type                 string   `json:"type"`
	Parent               string   `json:"parent,omitempty"`
	Title                string   `json:"title"`
	Priority             string   `json:"priority,omitempty"`
	Scope                []string `json:"scope,omitempty"`
	EstComplexity        string   `json:"estimated_complexity,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
}

// ComputeReady applies the 4-rule gate and returns a priority-sorted ready queue.
// `now` is the current Unix epoch for TTL evaluation.
func ComputeReady(index materialize.Index, issues map[string]*materialize.Issue, now ...int64) []ReadyEntry {
	var currentTime int64
	if len(now) > 0 {
		currentTime = now[0]
	} else {
		currentTime = time.Now().Unix()
	}
	var ready []ReadyEntry

	for id, entry := range index {
		if entry.Type != "task" {
			continue
		}
		// Rule 1: status == open
		if entry.Status != ops.StatusOpen {
			continue
		}
		// Rule 2: all blocked_by have status == merged
		if !allBlockersMerged(entry.BlockedBy, index) {
			continue
		}
		// Rule 3: parent is in-progress (or null)
		if entry.Parent != "" {
			parentEntry, ok := index[entry.Parent]
			if !ok || parentEntry.Status != ops.StatusInProgress {
				continue
			}
		}
		// Rule 4: not claimed, or claim expired (TTL exceeded)
		issue := issues[id]
		if issue != nil && issue.ClaimedBy != "" {
			if !claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, currentTime) {
				continue // actively claimed
			}
		}

		re := ReadyEntry{
			Issue:         id,
			Type:          entry.Type,
			Parent:        entry.Parent,
			Title:         entry.Title,
			Priority:      issue.Priority,
			Scope:         entry.Scope,
			EstComplexity: issue.EstComplexity,
		}

		if issue != nil && issue.Provenance.Confidence == "inferred" {
			re.RequiresConfirmation = true
		}

		ready = append(ready, re)
	}

	sortReady(ready, index)
	return ready
}

func allBlockersMerged(blockers []string, index materialize.Index) bool {
	for _, bid := range blockers {
		entry, ok := index[bid]
		if !ok || entry.Status != ops.StatusMerged {
			return false
		}
	}
	return true
}

var priorityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
	"":         4,
}

func sortReady(entries []ReadyEntry, index materialize.Index) {
	sort.SliceStable(entries, func(i, j int) bool {
		// 1. Explicit priority (higher first)
		pi := priorityOrder[entries[i].Priority]
		pj := priorityOrder[entries[j].Priority]
		if pi != pj {
			return pi < pj
		}

		// 2. Depth in hierarchy (deeper first)
		di := depth(entries[i].Issue, index)
		dj := depth(entries[j].Issue, index)
		if di != dj {
			return di > dj
		}

		// 3. Number of downstream unblocked (higher first)
		bi := len(index[entries[i].Issue].Blocks)
		bj := len(index[entries[j].Issue].Blocks)
		if bi != bj {
			return bi > bj
		}

		// 4. ID (lexicographic — stable tiebreaker)
		return entries[i].Issue < entries[j].Issue
	})
}

func depth(id string, index materialize.Index) int {
	d := 0
	for {
		entry, ok := index[id]
		if !ok || entry.Parent == "" {
			return d
		}
		id = entry.Parent
		d++
		if d > 20 {
			return d // safety cap
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/ready/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ready/
git commit -m "feat(ready): implement 4-rule ready gate and priority sort"
```

---

### Task 17: `trls ready` CLI command

**Files:**
- Create: `cmd/trellis/ready.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write test for ready command**

Add to `cmd/trellis/main_test.go`:

```go
func TestReadyCommand_EmptyRepo(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"ready", "--repo", repo})

	err := cmd2.Execute()
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Create ready.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ready"
	"github.com/spf13/cobra"
)

func newReadyCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show tasks ready to be claimed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			// Materialize first
			if _, err := materialize.Materialize(issuesDir, true); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			// Load state
			index, err := materialize.LoadIndex(issuesDir + "/state/index.json")
			if err != nil {
				return err
			}

			// Load all issues for full data
			issues := make(map[string]*materialize.Issue)
			for id := range index {
				issue, err := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
				if err == nil {
					issues[id] = &issue
				}
			}

			entries := ready.ComputeReady(index, issues)

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" {
				data, _ := json.MarshalIndent(entries, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				if len(entries) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
					return nil
				}
				for _, e := range entries {
					conf := ""
					if e.RequiresConfirmation {
						conf = " [requires confirmation]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  (%s)%s\n", e.Issue, e.Title, e.Priority, conf)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	return cmd
}
```

- [ ] **Step 3: Register and run tests**

Add `root.AddCommand(newReadyCmd())` in `newRootCmd()`.

Run: `cd /home/brian/development/trellis && go test ./cmd/trellis/ -run TestReady -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/ready.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls ready command"
```

---

### Task 18: Core ops CLI — create, note, link, decision, heartbeat

**Files:**
- Create: `cmd/trellis/create.go`
- Create: `cmd/trellis/note.go`
- Create: `cmd/trellis/link.go`
- Create: `cmd/trellis/decision.go`
- Create: `cmd/trellis/heartbeat.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write test for create command**

```go
func TestCreateCommand(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Fix bug", "--type", "task", "--id", "task-99"})

	err := cmd2.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-99")
}
```

- [ ] **Step 2: Create helper for resolving common flags across write commands**

Create a shared helper function in `cmd/trellis/helpers.go`:

```go
package main

import (
	"fmt"
	"time"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
)

// resolveWorkerAndLog returns the worker ID and log path for appending ops.
func resolveWorkerAndLog(repoPath string) (string, string, error) {
	workerID, err := worker.GetWorkerID(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	logPath := fmt.Sprintf("%s/.issues/ops/%s.log", repoPath, workerID)
	return workerID, logPath, nil
}

func nowEpoch() int64 {
	return time.Now().Unix()
}
```

- [ ] **Step 3: Create create.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var repoPath, title, nodeType, parent, id, priority, dod string
	var scope []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new work item",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			if id == "" {
				id = fmt.Sprintf("%s-%d", nodeType, nowEpoch())
			}

			op := ops.Op{
				Type:      ops.OpCreate,
				TargetID:  id,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Title:            title,
					NodeType:         nodeType,
					Parent:           parent,
					Scope:            scope,
					Priority:         priority,
					DefinitionOfDone: dod,
				},
			}

			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}

			result := map[string]string{"id": id, "status": "created"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&nodeType, "type", "task", "item type: epic, story, task")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node ID")
	cmd.Flags().StringVar(&id, "id", "", "explicit ID (auto-generated if empty)")
	cmd.Flags().StringVar(&priority, "priority", "", "priority: critical, high, medium, low")
	cmd.Flags().StringVar(&dod, "dod", "", "definition of done")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "file scope globs")
	cmd.MarkFlagRequired("title")

	return cmd
}
```

- [ ] **Step 4: Create note.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newNoteCmd() *cobra.Command {
	var repoPath, issueID, msg string

	cmd := &cobra.Command{
		Use:   "note",
		Short: "Add a note to an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Msg: msg}}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "note": "added"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&msg, "msg", "", "note message")
	cmd.MarkFlagRequired("issue")
	cmd.MarkFlagRequired("msg")
	return cmd
}
```

- [ ] **Step 5: Create link.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var repoPath, sourceID, dep, rel string

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Add a dependency link between issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpLink, TargetID: sourceID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: rel}}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"source": sourceID, "dep": dep, "rel": rel}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID")
	cmd.Flags().StringVar(&rel, "rel", "blocked_by", "relationship type")
	cmd.MarkFlagRequired("source")
	cmd.MarkFlagRequired("dep")
	return cmd
}
```

- [ ] **Step 6: Create decision.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newDecisionCmd() *cobra.Command {
	var repoPath, issueID, topic, choice, rationale string
	var affects []string

	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Record an architectural decision",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpDecision, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Topic: topic, Choice: choice,
					Rationale: rationale, Affects: affects}}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "topic": topic, "choice": choice}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&topic, "topic", "", "decision topic")
	cmd.Flags().StringVar(&choice, "choice", "", "chosen option")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why this choice")
	cmd.Flags().StringSliceVar(&affects, "affects", nil, "affected scope globs")
	cmd.MarkFlagRequired("issue")
	cmd.MarkFlagRequired("topic")
	cmd.MarkFlagRequired("choice")
	return cmd
}
```

- [ ] **Step 7: Create heartbeat.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var repoPath, issueID string

	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Send heartbeat for an active claim",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpHeartbeat, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "heartbeat": "sent"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 5: Register all commands in main.go**

```go
root.AddCommand(newCreateCmd())
root.AddCommand(newNoteCmd())
root.AddCommand(newLinkCmd())
root.AddCommand(newDecisionCmd())
root.AddCommand(newHeartbeatCmd())
```

- [ ] **Step 6: Run all tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/trellis/create.go cmd/trellis/note.go cmd/trellis/link.go cmd/trellis/decision.go cmd/trellis/heartbeat.go cmd/trellis/helpers.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add create, note, link, decision, and heartbeat commands"
```

---

### Task 19: Status transitions — transition, reopen, merged stub

**Files:**
- Create: `cmd/trellis/transition.go`
- Create: `cmd/trellis/reopen.go`
- Create: `cmd/trellis/merged.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write test for transition command**

```go
// setupRepoWithTask creates a temp repo, runs trls init, and creates a test task.
func setupRepoWithTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	return repo
}

func TestTransitionCommand(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Fixed"})

	err := cmd.Execute()
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Create transition.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newTransitionCmd() *cobra.Command {
	var repoPath, issueID, to, outcome, branch, pr string

	cmd := &cobra.Command{
		Use:   "transition",
		Short: "Transition an issue to a new status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			op := ops.Op{
				Type:      ops.OpTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					To:      to,
					Outcome: outcome,
					Branch:  branch,
					PR:      pr,
				},
			}

			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}

			result := map[string]string{"issue": issueID, "status": to}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&to, "to", "", "target status")
	cmd.Flags().StringVar(&outcome, "outcome", "", "outcome description")
	cmd.Flags().StringVar(&branch, "branch", "", "feature branch name")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number")
	cmd.MarkFlagRequired("issue")
	cmd.MarkFlagRequired("to")

	return cmd
}
```

- [ ] **Step 3: Create reopen.go**

`trls reopen` is a convenience wrapper that emits `transition --to open`:

```go
package main

import (
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newReopenCmd() *cobra.Command {
	var repoPath, issueID string

	cmd := &cobra.Command{
		Use:   "reopen",
		Short: "Reopen a done or blocked issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusOpen},
			}
			return ops.AppendOp(logPath, op)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to reopen")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 4: Create merged.go (stub)**

Per spec: `trls merged` is a stub in E1 single-branch mode — auto-handled by materializer.

```go
package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newMergedCmd() *cobra.Command {
	var repoPath, issueID string

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark an issue as merged (no-op in single-branch mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusMerged},
			}

			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for compatibility.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 5: Register commands, run all tests**

```go
root.AddCommand(newTransitionCmd())
root.AddCommand(newReopenCmd())
root.AddCommand(newMergedCmd())
```

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/transition.go cmd/trellis/reopen.go cmd/trellis/merged.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add transition, reopen, and merged stub commands"
```

---

## Chunk 5: Claim System (E1-S4)

### Task 20: Claim logic — race resolution, TTL, post-claim verify

**Files:**
- Create: `internal/claim/claim.go`
- Create: `internal/claim/claim_test.go`

- [ ] **Step 1: Write test for claim race resolution**

Create `internal/claim/claim_test.go`:

```go
package claim

import (
	"testing"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestResolveClaimRace_FirstTimestampWins(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}

	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestResolveClaimRace_LexicographicTiebreaker(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}

	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID) // lexicographic: a < b
}

func TestIsClaimStale(t *testing.T) {
	// Claim at t=100, TTL=60, no heartbeat
	assert.True(t, IsClaimStale(100, 0, 60, 161))   // past TTL
	assert.False(t, IsClaimStale(100, 0, 60, 159))   // within TTL

	// With heartbeat at t=150
	assert.False(t, IsClaimStale(100, 150, 60, 200)) // within TTL of heartbeat
	assert.True(t, IsClaimStale(100, 150, 60, 211))  // past TTL of heartbeat
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/claim/ -v`
Expected: FAIL

- [ ] **Step 3: Create claim.go**

```go
package claim

import (
	"github.com/scullxbones/trellis/internal/ops"
)

// ResolveClaim resolves a claim race: earliest timestamp wins,
// lexicographic worker ID as tiebreaker.
func ResolveClaim(claims []ops.Op) ops.Op {
	if len(claims) == 0 {
		return ops.Op{}
	}
	winner := claims[0]
	for _, c := range claims[1:] {
		if c.Timestamp < winner.Timestamp ||
			(c.Timestamp == winner.Timestamp && c.WorkerID < winner.WorkerID) {
			winner = c
		}
	}
	return winner
}

// IsClaimStale checks if a claim has expired based on TTL and heartbeat.
func IsClaimStale(claimedAt, lastHeartbeat int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := claimedAt
	if lastHeartbeat > lastActivity {
		lastActivity = lastHeartbeat
	}
	ttlSeconds := int64(ttlMinutes) * 60
	return now > lastActivity+ttlSeconds
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/claim/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/claim/
git commit -m "feat(claim): add claim race resolution and stale claim detection"
```

---

### Task 21: Scope overlap advisory

**Files:**
- Create: `internal/claim/overlap.go`
- Modify: `internal/claim/claim_test.go`

- [ ] **Step 1: Write test for scope overlap detection**

```go
func TestScopeOverlap(t *testing.T) {
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}
```

- [ ] **Step 2: Create overlap.go**

```go
package claim

import "path/filepath"

// ScopesOverlap checks if two scope glob lists have any overlap.
func ScopesOverlap(scopeA, scopeB []string) bool {
	for _, a := range scopeA {
		for _, b := range scopeB {
			if globOverlaps(a, b) {
				return true
			}
		}
	}
	return false
}

// globOverlaps checks if two glob patterns could match the same file.
// Simple approach: check if either pattern matches the other literally,
// or if they share a common prefix.
func globOverlaps(a, b string) bool {
	// If either matches the other literally
	if matched, _ := filepath.Match(a, b); matched {
		return true
	}
	if matched, _ := filepath.Match(b, a); matched {
		return true
	}
	// For ** patterns: check common directory prefix
	dirA := extractDir(a)
	dirB := extractDir(b)
	if dirA == "" || dirB == "" {
		return false
	}
	// One is prefix of the other
	return hasPrefix(dirA, dirB) || hasPrefix(dirB, dirA)
}

func extractDir(pattern string) string {
	for i := len(pattern) - 1; i >= 0; i-- {
		if pattern[i] == '/' {
			return pattern[:i]
		}
	}
	return ""
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
```

- [ ] **Step 3: Run tests, commit**

Run: `cd /home/brian/development/trellis && go test ./internal/claim/ -v`
Expected: PASS

```bash
git add internal/claim/overlap.go internal/claim/claim_test.go
git commit -m "feat(claim): add scope overlap detection for claim advisory"
```

---

### Task 22: `trls claim` CLI command

**Files:**
- Create: `cmd/trellis/claim.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write test for claim command**

```go
func TestClaimCommand(t *testing.T) {
	repo := setupRepoWithTask(t) // creates init + task-01

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-01")
}
```

- [ ] **Step 2: Create claim.go**

The claim command:
1. Materializes state
2. Checks if task is ready (open, not claimed, blockers merged)
3. Checks if task requires confirmation (inferred nodes cannot be claimed)
4. Checks scope overlap with other active claims → advisory warning + auto-notes
5. Appends claim op
6. Outputs JSON result

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/claim"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newClaimCmd() *cobra.Command {
	var repoPath, issueID string
	var ttl int

	cmd := &cobra.Command{
		Use:   "claim",
		Short: "Claim a ready task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			// Materialize
			if _, err := materialize.Materialize(issuesDir, true); err != nil {
				return err
			}

			// Load issue
			issue, err := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, issueID))
			if err != nil {
				return fmt.Errorf("issue %s not found: %w", issueID, err)
			}

			// Check inferred
			if issue.Provenance.Confidence == "inferred" {
				return fmt.Errorf("cannot claim %s: node has confidence=inferred — wait for a human to confirm it", issueID)
			}

			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			// Check scope overlap with other active claims
			index, _ := materialize.LoadIndex(issuesDir + "/state/index.json")
			for id, entry := range index {
				if id == issueID || (entry.Status != "claimed" && entry.Status != "in-progress") {
					continue
				}
				if claim.ScopesOverlap(issue.Scope, entry.Scope) {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: scope overlap with %s (%s)\n", id, entry.Title)
					// Auto-note on both tasks
					noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", id)}}
					ops.AppendOp(logPath, noteOp)
					noteOp2 := ops.Op{Type: ops.OpNote, TargetID: id, Timestamp: nowEpoch(),
						WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", issueID)}}
					ops.AppendOp(logPath, noteOp2)
				}
			}

			// Append claim
			op := ops.Op{
				Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{TTL: ttl},
			}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}

			result := map[string]interface{}{"issue": issueID, "claimed_by": workerID, "ttl": ttl}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to claim")
	cmd.Flags().IntVar(&ttl, "ttl", 60, "claim TTL in minutes")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 3: Register and run tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/claim.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls claim with scope overlap advisory and inferred node blocking"
```

---

## Chunk 6: Context Assembly (E1-S5)

### Task 23: 7-layer context assembly algorithm

**Files:**
- Create: `internal/context/assemble.go`
- Create: `internal/context/truncate.go`
- Create: `internal/context/context_test.go`

- [ ] **Step 1: Write tests for context assembly**

Create `internal/context/context_test.go`:

```go
package context

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleContext_CoreSpec(t *testing.T) {
	issue := &materialize.Issue{
		ID: "task-01", Type: "task", Status: "open", Title: "Fix auth",
		Priority: "high", DefinitionOfDone: "Tests pass",
		Scope: []string{"src/auth/**"},
	}
	index := materialize.Index{}
	issues := map[string]*materialize.Issue{"task-01": issue}

	ctx := Assemble("task-01", index, issues, 1600)

	require.NotNil(t, ctx)
	assert.Equal(t, "task-01", ctx.ID)
	assert.Equal(t, "Fix auth", ctx.Title)
	assert.Equal(t, "Tests pass", ctx.DefinitionOfDone)
	assert.Contains(t, ctx.Scope, "src/auth/**")
}

func TestAssembleContext_BlockerOutcomes(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Type: "task", Status: "open", BlockedBy: []string{"task-02"}},
		"task-02": {ID: "task-02", Type: "task", Status: "merged", Title: "Setup DB", Outcome: "Added migrations"},
	}
	index := materialize.Index{
		"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
		"task-02": {Status: "merged", Title: "Setup DB", Outcome: "Added migrations"},
	}

	ctx := Assemble("task-01", index, issues, 1600)
	assert.Len(t, ctx.BlockerOutcomes, 1)
	assert.Equal(t, "Added migrations", ctx.BlockerOutcomes[0].Outcome)
}

func TestAssembleContext_ParentChain(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01":  {ID: "task-01", Type: "task", Parent: "story-01"},
		"story-01": {ID: "story-01", Type: "story", Title: "Auth story", Parent: "epic-01"},
		"epic-01":  {ID: "epic-01", Type: "epic", Title: "Auth epic"},
	}
	index := materialize.Index{
		"task-01":  {Parent: "story-01"},
		"story-01": {Parent: "epic-01", Type: "story", Title: "Auth story"},
		"epic-01":  {Type: "epic", Title: "Auth epic"},
	}

	ctx := Assemble("task-01", index, issues, 1600)
	assert.Len(t, ctx.ParentChain, 2)
	assert.Equal(t, "story-01", ctx.ParentChain[0].ID)
	assert.Equal(t, "epic-01", ctx.ParentChain[1].ID)
}

func TestAssembleContext_Truncation(t *testing.T) {
	issue := &materialize.Issue{
		ID: "task-01", Type: "task", Status: "open", Title: "T",
		DefinitionOfDone: "D", Scope: []string{"src/**"},
	}
	// Create many notes to exceed budget
	for i := 0; i < 100; i++ {
		issue.Notes = append(issue.Notes, materialize.Note{Msg: "A long note that takes up space in the context budget and should eventually be truncated."})
	}
	issues := map[string]*materialize.Issue{"task-01": issue}
	index := materialize.Index{"task-01": {Status: "open"}}

	ctx := Assemble("task-01", index, issues, 50) // very small budget
	assert.True(t, ctx.BudgetInfo.Truncated)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/context/ -v`
Expected: FAIL

- [ ] **Step 3: Create assemble.go**

```go
package context

import (
	"github.com/scullxbones/trellis/internal/materialize"
)

// RenderedContext is the output of the context assembly algorithm.
type RenderedContext struct {
	ID               string           `json:"id"`
	Type             string           `json:"type"`
	Status           string           `json:"status"`
	Title            string           `json:"title"`
	Priority         string           `json:"priority,omitempty"`
	DefinitionOfDone string           `json:"definition_of_done,omitempty"`
	Scope            []string         `json:"scope,omitempty"`
	Acceptance       interface{}      `json:"acceptance,omitempty"`
	Snippets         interface{}      `json:"snippets,omitempty"`
	BlockerOutcomes  []BlockerOutcome `json:"blocker_outcomes,omitempty"`
	ParentChain      []ParentSummary  `json:"parent_chain,omitempty"`
	Decisions        []DecisionRef    `json:"decisions,omitempty"`
	Notes            []NoteRef        `json:"notes,omitempty"`
	SiblingOutcomes  []SiblingOutcome `json:"sibling_outcomes,omitempty"`
	BudgetInfo       BudgetInfo       `json:"budget_info"`
}

type BlockerOutcome struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

type ParentSummary struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type DecisionRef struct {
	Topic     string `json:"topic"`
	Choice    string `json:"choice"`
	Rationale string `json:"rationale"`
}

type NoteRef struct {
	WorkerID  string `json:"worker_id"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
}

type SiblingOutcome struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

type BudgetInfo struct {
	BudgetTokens    int  `json:"budget_tokens"`
	EstimatedTokens int  `json:"estimated_tokens"`
	Truncated       bool `json:"truncated"`
}

// Assemble builds the 7-layer context for a given issue.
func Assemble(issueID string, index materialize.Index, issues map[string]*materialize.Issue, budget int) *RenderedContext {
	issue, ok := issues[issueID]
	if !ok {
		return nil
	}

	ctx := &RenderedContext{
		ID:               issue.ID,
		Type:             issue.Type,
		Status:           issue.Status,
		Title:            issue.Title,
		Priority:         issue.Priority,
		DefinitionOfDone: issue.DefinitionOfDone,
		Scope:            issue.Scope,
		Acceptance:       issue.Acceptance,
		Snippets:         issue.Context,
	}

	// Layer 3: Blocker outcomes
	for _, depID := range issue.BlockedBy {
		dep, ok := issues[depID]
		if !ok {
			continue
		}
		if (dep.Status == "done" || dep.Status == "merged") && dep.Outcome != "" {
			ctx.BlockerOutcomes = append(ctx.BlockerOutcomes, BlockerOutcome{
				ID: dep.ID, Title: dep.Title, Outcome: dep.Outcome,
			})
		}
	}

	// Layer 4: Parent chain (max depth 5)
	parentID := issue.Parent
	for depth := 0; parentID != "" && depth < 5; depth++ {
		parent, ok := issues[parentID]
		if !ok {
			break
		}
		ctx.ParentChain = append(ctx.ParentChain, ParentSummary{
			ID: parent.ID, Type: parent.Type, Title: parent.Title, Status: parent.Status,
		})
		parentID = parent.Parent
	}

	// Layer 5: Decisions (scope-overlapping — simplified for E1: all decisions on this issue)
	for _, d := range issue.Decisions {
		ctx.Decisions = append(ctx.Decisions, DecisionRef{
			Topic: d.Topic, Choice: d.Choice, Rationale: d.Rationale,
		})
	}

	// Layer 6: Notes (newest first)
	for i := len(issue.Notes) - 1; i >= 0; i-- {
		ctx.Notes = append(ctx.Notes, NoteRef{
			WorkerID: issue.Notes[i].WorkerID, Timestamp: issue.Notes[i].Timestamp, Msg: issue.Notes[i].Msg,
		})
	}

	// Layer 7: Sibling outcomes (same parent, done/merged, scope overlap)
	if issue.Parent != "" {
		if parent, ok := issues[issue.Parent]; ok {
			for _, sibID := range parent.Children {
				if sibID == issueID {
					continue
				}
				sib, ok := issues[sibID]
				if !ok || (sib.Status != "done" && sib.Status != "merged") || sib.Outcome == "" {
					continue
				}
				ctx.SiblingOutcomes = append(ctx.SiblingOutcomes, SiblingOutcome{
					ID: sib.ID, Title: sib.Title, Outcome: sib.Outcome,
				})
			}
		}
	}

	// Truncation
	ctx.BudgetInfo = Truncate(ctx, budget)

	return ctx
}
```

- [ ] **Step 4: Create truncate.go**

```go
package context

import (
	"encoding/json"
)

// Truncate removes lowest-priority truncatable layers to fit within token budget.
// Fixed layers (core spec, snippets) are never removed.
// Priority order (lowest priority removed first):
//   5: sibling outcomes
//   4: notes
//   3: decisions
//   2: parent chain
//   1: blocker outcomes
func Truncate(ctx *RenderedContext, budgetTokens int) BudgetInfo {
	estimated := estimateTokens(ctx)
	info := BudgetInfo{
		BudgetTokens:    budgetTokens,
		EstimatedTokens: estimated,
	}

	if estimated <= budgetTokens {
		return info
	}

	// Remove layers in reverse priority order
	// Priority 5: sibling outcomes
	if estimateTokens(ctx) > budgetTokens && len(ctx.SiblingOutcomes) > 0 {
		for len(ctx.SiblingOutcomes) > 0 && estimateTokens(ctx) > budgetTokens {
			ctx.SiblingOutcomes = ctx.SiblingOutcomes[:len(ctx.SiblingOutcomes)-1]
		}
	}

	// Priority 4: notes (remove oldest first = last in our newest-first list)
	if estimateTokens(ctx) > budgetTokens && len(ctx.Notes) > 0 {
		for len(ctx.Notes) > 0 && estimateTokens(ctx) > budgetTokens {
			ctx.Notes = ctx.Notes[:len(ctx.Notes)-1]
		}
	}

	// Priority 3: decisions
	if estimateTokens(ctx) > budgetTokens && len(ctx.Decisions) > 0 {
		for len(ctx.Decisions) > 0 && estimateTokens(ctx) > budgetTokens {
			ctx.Decisions = ctx.Decisions[:len(ctx.Decisions)-1]
		}
	}

	// Priority 2: parent chain (remove farthest ancestor first)
	if estimateTokens(ctx) > budgetTokens && len(ctx.ParentChain) > 0 {
		for len(ctx.ParentChain) > 0 && estimateTokens(ctx) > budgetTokens {
			ctx.ParentChain = ctx.ParentChain[:len(ctx.ParentChain)-1]
		}
	}

	// Priority 1: blocker outcomes
	if estimateTokens(ctx) > budgetTokens && len(ctx.BlockerOutcomes) > 0 {
		for len(ctx.BlockerOutcomes) > 0 && estimateTokens(ctx) > budgetTokens {
			ctx.BlockerOutcomes = ctx.BlockerOutcomes[:len(ctx.BlockerOutcomes)-1]
		}
	}

	info.EstimatedTokens = estimateTokens(ctx)
	info.Truncated = true
	return info
}

// estimateTokens uses chars/4 proxy per spec.
func estimateTokens(ctx *RenderedContext) int {
	data, _ := json.Marshal(ctx)
	return len(data) / 4
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/context/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/context/
git commit -m "feat(context): implement 7-layer context assembly with priority-based truncation"
```

---

### Task 24: `trls render-context` CLI command

**Files:**
- Create: `cmd/trellis/render_context.go`
- Create: `internal/context/render.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write test for render-context command**

```go
func TestRenderContextCommand(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"render-context", "--repo", repo, "--issue", "task-01", "--format", "json"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "task-01")
}
```

- [ ] **Step 2: Create render.go**

```go
package context

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderAgent outputs the context as JSON (for AI agents).
func RenderAgent(ctx *RenderedContext) ([]byte, error) {
	return json.MarshalIndent(ctx, "", "  ")
}

// RenderHuman outputs the context as plain text (for humans).
// E1 basic version — E3-S2 upgrades to Glamour markdown rendering.
func RenderHuman(ctx *RenderedContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== %s: %s ===\n", ctx.ID, ctx.Title)
	fmt.Fprintf(&b, "Type: %s | Status: %s | Priority: %s\n\n", ctx.Type, ctx.Status, ctx.Priority)

	if ctx.DefinitionOfDone != "" {
		fmt.Fprintf(&b, "Definition of Done:\n  %s\n\n", ctx.DefinitionOfDone)
	}

	if len(ctx.Scope) > 0 {
		fmt.Fprintf(&b, "Scope:\n")
		for _, s := range ctx.Scope {
			fmt.Fprintf(&b, "  %s\n", s)
		}
		b.WriteString("\n")
	}

	if len(ctx.BlockerOutcomes) > 0 {
		fmt.Fprintf(&b, "Blocker Outcomes:\n")
		for _, bo := range ctx.BlockerOutcomes {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", bo.ID, bo.Title, bo.Outcome)
		}
		b.WriteString("\n")
	}

	if len(ctx.ParentChain) > 0 {
		fmt.Fprintf(&b, "Parent Chain:\n")
		for _, p := range ctx.ParentChain {
			fmt.Fprintf(&b, "  %s (%s) — %s [%s]\n", p.ID, p.Type, p.Title, p.Status)
		}
		b.WriteString("\n")
	}

	if len(ctx.Notes) > 0 {
		fmt.Fprintf(&b, "Notes:\n")
		for _, n := range ctx.Notes {
			fmt.Fprintf(&b, "  [%s] %s\n", n.WorkerID, n.Msg)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Budget: %d/%d tokens", ctx.BudgetInfo.EstimatedTokens, ctx.BudgetInfo.BudgetTokens)
	if ctx.BudgetInfo.Truncated {
		b.WriteString(" (truncated)")
	}
	b.WriteString("\n")

	return b.String()
}
```

- [ ] **Step 3: Create render_context.go CLI command**

```go
package main

import (
	"fmt"

	ctx "github.com/scullxbones/trellis/internal/context"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newRenderContextCmd() *cobra.Command {
	var repoPath, issueID string
	var budget int
	var raw bool

	cmd := &cobra.Command{
		Use:   "render-context",
		Short: "Assemble and render context for a task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			if _, err := materialize.Materialize(issuesDir, true); err != nil {
				return err
			}

			index, err := materialize.LoadIndex(issuesDir + "/state/index.json")
			if err != nil {
				return err
			}

			issues := make(map[string]*materialize.Issue)
			for id := range index {
				issue, err := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
				if err == nil {
					issues[id] = &issue
				}
			}

			if raw {
				budget = 999999 // effectively unlimited
			}

			rendered := ctx.Assemble(issueID, index, issues, budget)
			if rendered == nil {
				return fmt.Errorf("issue %s not found", issueID)
			}

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" {
				data, _ := ctx.RenderAgent(rendered)
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), ctx.RenderHuman(rendered))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().IntVar(&budget, "budget", 1600, "token budget")
	cmd.Flags().BoolVar(&raw, "raw", false, "bypass all truncation")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 4: Register and run tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/render_context.go internal/context/render.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat(cli): add trls render-context with agent JSON and human text output"
```

---

## Chunk 7: Decomposition Workflow + Validate (E1-S8)

### Task 25: Plan file format v1 parsing

**Files:**
- Create: `internal/decompose/plan.go`
- Create: `internal/decompose/decompose_test.go`

- [ ] **Step 1: Write test for plan file parsing**

Create `internal/decompose/decompose_test.go`:

```go
package decompose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlanFile(t *testing.T) {
	plan := Plan{
		Version: "1",
		BatchID: "batch-001",
		Nodes: []PlanNode{
			{ID: "epic-01", Type: "epic", Title: "Auth Epic"},
			{ID: "story-01", Type: "story", Title: "Login Story", Parent: "epic-01"},
			{ID: "task-01", Type: "task", Title: "Impl login", Parent: "story-01",
				Scope: []string{"src/auth/**"}, DefinitionOfDone: "Tests pass",
				Acceptance: json.RawMessage(`[{"type":"test_passes","pattern":"tests/auth/**"}]`)},
		},
	}

	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	data, _ := json.MarshalIndent(plan, "", "  ")
	os.WriteFile(planPath, data, 0644)

	loaded, err := LoadPlan(planPath)
	require.NoError(t, err)
	assert.Equal(t, "1", loaded.Version)
	assert.Len(t, loaded.Nodes, 3)
	assert.Equal(t, "epic-01", loaded.Nodes[0].ID)
}

func TestValidatePlan_CycleDetection(t *testing.T) {
	plan := Plan{
		Nodes: []PlanNode{
			{ID: "task-01", Type: "task", Title: "A", BlockedBy: []string{"task-02"}},
			{ID: "task-02", Type: "task", Title: "B", BlockedBy: []string{"task-01"}},
		},
	}

	errs := ValidatePlan(plan)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "cycle")
}

func TestValidatePlan_OrphanParent(t *testing.T) {
	plan := Plan{
		Nodes: []PlanNode{
			{ID: "task-01", Type: "task", Title: "A", Parent: "nonexistent"},
		},
	}

	errs := ValidatePlan(plan)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "parent")
}

func TestValidatePlan_MissingRequiredFields(t *testing.T) {
	plan := Plan{
		Nodes: []PlanNode{
			{ID: "task-01", Type: "task", Title: "A"}, // missing scope, acceptance, dod for tasks
		},
	}

	errs := ValidatePlan(plan)
	assert.NotEmpty(t, errs)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/decompose/ -v`
Expected: FAIL

- [ ] **Step 3: Create plan.go**

```go
package decompose

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/scullxbones/trellis/internal/dag"
)

type Plan struct {
	Version string     `json:"version"`
	BatchID string     `json:"batch_id"`
	SHA     string     `json:"sha,omitempty"`
	Nodes   []PlanNode `json:"nodes"`
}

type PlanNode struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"` // epic, story, task
	Title            string          `json:"title"`
	Parent           string          `json:"parent,omitempty"`
	BlockedBy        []string        `json:"blocked_by,omitempty"`
	Scope            []string        `json:"scope,omitempty"`
	DefinitionOfDone string          `json:"definition_of_done,omitempty"`
	Acceptance       json.RawMessage `json:"acceptance,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	SourceCitation   json.RawMessage `json:"source_citation,omitempty"`
	Priority         string          `json:"priority,omitempty"`
	EstComplexity    string          `json:"estimated_complexity,omitempty"`
}

func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, fmt.Errorf("parse plan: %w", err)
	}
	return plan, nil
}

// ValidatePlan checks structural integrity of a plan (E1-E12 validation rules).
func ValidatePlan(plan Plan) []error {
	var errs []error

	nodeIDs := make(map[string]bool)
	validTypes := map[string]bool{"epic": true, "story": true, "task": true}

	// E1: Duplicate ID check
	for _, n := range plan.Nodes {
		if nodeIDs[n.ID] {
			errs = append(errs, fmt.Errorf("E1: duplicate node ID %s", n.ID))
		}
		nodeIDs[n.ID] = true
	}

	// E2: Valid type values
	for _, n := range plan.Nodes {
		if !validTypes[n.Type] {
			errs = append(errs, fmt.Errorf("E2: node %s has invalid type %q (must be epic, story, or task)", n.ID, n.Type))
		}
	}

	// E3: Non-empty title
	for _, n := range plan.Nodes {
		if n.Title == "" {
			errs = append(errs, fmt.Errorf("E3: node %s missing title", n.ID))
		}
	}

	// E4: Orphan parent references
	for _, n := range plan.Nodes {
		if n.Parent != "" && !nodeIDs[n.Parent] {
			errs = append(errs, fmt.Errorf("E4: node %s references unknown parent %s", n.ID, n.Parent))
		}
	}

	// E5: blocked_by references exist in plan
	for _, n := range plan.Nodes {
		for _, dep := range n.BlockedBy {
			if !nodeIDs[dep] {
				errs = append(errs, fmt.Errorf("E5: node %s has blocked_by reference to unknown node %s", n.ID, dep))
			}
		}
	}

	// E6: Required fields for tasks (scope, acceptance, definition_of_done)
	for _, n := range plan.Nodes {
		if n.Type == "task" {
			if len(n.Scope) == 0 {
				errs = append(errs, fmt.Errorf("E6: task %s missing scope", n.ID))
			}
			if n.DefinitionOfDone == "" {
				errs = append(errs, fmt.Errorf("E6: task %s missing definition_of_done", n.ID))
			}
			if len(n.Acceptance) == 0 || string(n.Acceptance) == "null" {
				errs = append(errs, fmt.Errorf("E6: task %s missing acceptance", n.ID))
			}
		}
	}

	// E8: Parent type hierarchy (epics contain stories, stories contain tasks)
	for _, n := range plan.Nodes {
		if n.Parent == "" {
			continue
		}
		for _, p := range plan.Nodes {
			if p.ID == n.Parent {
				if n.Type == "story" && p.Type != "epic" {
					errs = append(errs, fmt.Errorf("E8: story %s must have epic parent, got %s", n.ID, p.Type))
				}
				if n.Type == "task" && p.Type != "story" && p.Type != "epic" {
					errs = append(errs, fmt.Errorf("E8: task %s must have story or epic parent, got %s", n.ID, p.Type))
				}
				break
			}
		}
	}

	// E10: Cycle detection via DAG
	d := dag.New()
	for _, n := range plan.Nodes {
		d.AddNode(&dag.Node{
			ID:        n.ID,
			Title:     n.Title,
			Type:      n.Type,
			Parent:    n.Parent,
			BlockedBy: n.BlockedBy,
		})
	}
	if d.HasCycle() {
		errs = append(errs, fmt.Errorf("E10: plan contains a dependency cycle"))
	}

	return errs
}

// GenerateIDs assigns auto-generated IDs to nodes that have empty IDs.
func GenerateIDs(plan *Plan) {
	for i := range plan.Nodes {
		if plan.Nodes[i].ID == "" {
			plan.Nodes[i].ID = fmt.Sprintf("%s-%d", plan.Nodes[i].Type, i+1)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/decompose/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/decompose/
git commit -m "feat(decompose): add plan file format v1 parsing and structural validation"
```

---

### Task 26: decompose-apply — load plan into ops

**Files:**
- Create: `internal/decompose/apply.go`
- Modify: `internal/decompose/decompose_test.go`

- [ ] **Step 1: Write test for decompose-apply**

```go
func TestApplyPlan(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	os.MkdirAll(opsDir, 0755)

	plan := Plan{
		Version: "1",
		BatchID: "batch-001",
		Nodes: []PlanNode{
			{ID: "epic-01", Type: "epic", Title: "Auth Epic"},
			{ID: "task-01", Type: "task", Title: "Login", Parent: "epic-01",
				Scope: []string{"src/**"}, DefinitionOfDone: "Done",
				Acceptance: json.RawMessage(`[]`)},
		},
	}

	result, err := ApplyPlan(plan, opsDir, "worker-a1", false)
	require.NoError(t, err)
	assert.Equal(t, 2, result.NodesCreated)

	// Verify ops were written
	logOps, err := ops.ReadLog(filepath.Join(opsDir, "worker-a1.log"))
	require.NoError(t, err)
	assert.Len(t, logOps, 2)
}

func TestApplyPlan_Idempotent(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	os.MkdirAll(opsDir, 0755)

	plan := Plan{Version: "1", BatchID: "batch-001",
		Nodes: []PlanNode{{ID: "task-01", Type: "task", Title: "T",
			Scope: []string{"src/**"}, DefinitionOfDone: "D", Acceptance: json.RawMessage(`[]`)}}}

	ApplyPlan(plan, opsDir, "worker-a1", false)
	result, err := ApplyPlan(plan, opsDir, "worker-a1", false)
	require.NoError(t, err)
	assert.Equal(t, 0, result.NodesCreated) // idempotent — no duplicates
}

func TestApplyPlan_DryRun(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	os.MkdirAll(opsDir, 0755)

	plan := Plan{Version: "1", BatchID: "batch-001",
		Nodes: []PlanNode{{ID: "task-01", Type: "task", Title: "T",
			Scope: []string{"src/**"}, DefinitionOfDone: "D", Acceptance: json.RawMessage(`[]`)}}}

	result, err := ApplyPlan(plan, opsDir, "worker-a1", true) // dry run
	require.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)

	// No log file should be created
	_, err = os.Stat(filepath.Join(opsDir, "worker-a1.log"))
	assert.True(t, os.IsNotExist(err))
}
```

- [ ] **Step 2: Create apply.go**

```go
package decompose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scullxbones/trellis/internal/ops"
)

type ApplyResult struct {
	NodesCreated int
	NodesSkipped int
	DryRun       bool
}

// ApplyPlan converts plan nodes into create ops and link ops, written atomically.
// Idempotency: checks batch ID + plan SHA against existing ops.
func ApplyPlan(plan Plan, opsDir string, workerID string, dryRun bool) (ApplyResult, error) {
	// Validate first
	if errs := ValidatePlan(plan); len(errs) > 0 {
		return ApplyResult{}, fmt.Errorf("plan validation failed: %v", errs)
	}

	// Compute plan SHA for idempotency
	planData, _ := json.Marshal(plan)
	hash := sha256.Sum256(planData)
	planSHA := hex.EncodeToString(hash[:])

	// Check for prior application of same batch
	logPath := filepath.Join(opsDir, workerID+".log")
	existingOps, _ := ops.ReadLog(logPath)
	existingIDs := make(map[string]bool)
	for _, op := range existingOps {
		if op.Type == ops.OpCreate {
			existingIDs[op.TargetID] = true
		}
	}

	var createOps []ops.Op
	var linkOps []ops.Op
	now := time.Now().Unix()
	skipped := 0

	for i, node := range plan.Nodes {
		if existingIDs[node.ID] {
			skipped++
			continue
		}

		createOps = append(createOps, ops.Op{
			Type:      ops.OpCreate,
			TargetID:  node.ID,
			Timestamp: now + int64(i), // ensure unique timestamps
			WorkerID:  workerID,
			Payload: ops.Payload{
				Title:            node.Title,
				NodeType:         node.Type,
				Parent:           node.Parent,
				Scope:            node.Scope,
				DefinitionOfDone: node.DefinitionOfDone,
				Acceptance:       node.Acceptance,
				Context:          node.Context,
				SourceCitation:   node.SourceCitation,
				Priority:         node.Priority,
				EstComplexity:    node.EstComplexity,
			},
		})

		// Create link ops for blocked_by
		for _, dep := range node.BlockedBy {
			linkOps = append(linkOps, ops.Op{
				Type:      ops.OpLink,
				TargetID:  node.ID,
				Timestamp: now + int64(i),
				WorkerID:  workerID,
				Payload:   ops.Payload{Dep: dep, Rel: "blocked_by"},
			})
		}
	}

	result := ApplyResult{
		NodesCreated: len(createOps),
		NodesSkipped: skipped,
		DryRun:       dryRun,
	}

	if dryRun {
		return result, nil
	}

	// Atomic write: all ops to a single file write
	// Atomic write: buffer all ops and write in a single file append
	var allOpsToWrite []ops.Op
	allOpsToWrite = append(allOpsToWrite, createOps...)
	allOpsToWrite = append(allOpsToWrite, linkOps...)

	if err := ops.AppendOps(logPath, allOpsToWrite); err != nil {
		return result, fmt.Errorf("atomic write failed: %w", err)
	}

	// Record batch metadata for idempotency tracking
	metaOp := ops.Op{
		Type: ops.OpNote, TargetID: plan.BatchID, Timestamp: now,
		WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("decompose-apply batch=%s sha=%s nodes=%d", plan.BatchID, planSHA, len(createOps))},
	}
	ops.AppendOp(logPath, metaOp)

	return result, nil
}
```

- [ ] **Step 3: Run tests**

Run: `cd /home/brian/development/trellis && go test ./internal/decompose/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/decompose/apply.go internal/decompose/decompose_test.go
git commit -m "feat(decompose): add decompose-apply with idempotency and dry-run support"
```

---

### Task 27: decompose-revert + decompose-context stub

**Files:**
- Create: `internal/decompose/revert.go`
- Create: `internal/decompose/context_stub.go`
- Modify: `internal/decompose/decompose_test.go`

- [ ] **Step 1: Write test for revert**

```go
func TestRevertPlan(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	os.MkdirAll(opsDir, 0755)

	plan := Plan{Version: "1", BatchID: "batch-001",
		Nodes: []PlanNode{{ID: "task-01", Type: "task", Title: "T",
			Scope: []string{"src/**"}, DefinitionOfDone: "D", Acceptance: json.RawMessage(`[]`)}}}

	ApplyPlan(plan, opsDir, "worker-a1", false)
	err := RevertPlan(plan, opsDir, "worker-a1")
	require.NoError(t, err)

	// Should have cancel transition ops
	allOps, _ := ops.ReadLog(filepath.Join(opsDir, "worker-a1.log"))
	var cancelCount int
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.Payload.To == ops.StatusCancelled {
			cancelCount++
		}
	}
	assert.Equal(t, 1, cancelCount)
}
```

- [ ] **Step 2: Create revert.go**

```go
package decompose

import (
	"path/filepath"
	"time"

	"github.com/scullxbones/trellis/internal/ops"
)

// RevertPlan emits cancellation transition ops for all nodes in the plan.
func RevertPlan(plan Plan, opsDir string, workerID string) error {
	logPath := filepath.Join(opsDir, workerID+".log")
	now := time.Now().Unix()

	for i, node := range plan.Nodes {
		op := ops.Op{
			Type:      ops.OpTransition,
			TargetID:  node.ID,
			Timestamp: now + int64(i),
			WorkerID:  workerID,
			Payload:   ops.Payload{To: ops.StatusCancelled},
		}
		if err := ops.AppendOp(logPath, op); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 3: Write test for decompose-context stub**

```go
func TestDecomposeContextStub(t *testing.T) {
	dir := t.TempDir()

	// Create a source file
	os.WriteFile(filepath.Join(dir, "prd.md"), []byte("# PRD\nRequirements here"), 0644)

	result, err := DecomposeContext([]string{filepath.Join(dir, "prd.md")}, "", "")
	require.NoError(t, err)
	assert.Contains(t, result.PromptTemplate, "{{SOURCES}}")
	assert.Len(t, result.Sources, 1)
	assert.Contains(t, result.Sources[0].Content, "Requirements here")
}
```

- [ ] **Step 4: Create context_stub.go**

```go
package decompose

import (
	"encoding/json"
	"fmt"
	"os"
)

// DecomposeContextResult matches the output schema defined in architecture doc.
type DecomposeContextResult struct {
	PromptTemplate string                `json:"prompt_template"`
	Sources        []DecomposeSource     `json:"sources"`
	ExistingDAG    json.RawMessage       `json:"existing_dag,omitempty"`
	Constraints    json.RawMessage       `json:"constraints,omitempty"`
	PlanSchema     json.RawMessage       `json:"plan_schema,omitempty"`
}

type DecomposeSource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// DecomposeContext is the E1 stub implementation.
// Reads local filesystem sources, interpolates {{SOURCES}}.
// Full implementation in E3-S5.
func DecomposeContext(sources []string, templatePath string, format string) (DecomposeContextResult, error) {
	result := DecomposeContextResult{
		PromptTemplate: defaultDecomposeTemplate(),
		Sources:        []DecomposeSource{},
	}

	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			return result, fmt.Errorf("read source %s: %w", src, err)
		}
		result.Sources = append(result.Sources, DecomposeSource{
			Path:    src,
			Content: string(data),
		})
	}

	return result, nil
}

func defaultDecomposeTemplate() string {
	return `You are decomposing a project into an implementation plan.

## Source Documents

{{SOURCES}}

## Existing DAG

{{EXISTING_DAG}}

## Constraints

{{CONSTRAINTS}}

## Output Format

Produce a valid plan.json matching this schema:

{{PLAN_SCHEMA}}
`
}
```

- [ ] **Step 5: Run tests**

Run: `cd /home/brian/development/trellis && go test ./internal/decompose/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/decompose/revert.go internal/decompose/context_stub.go internal/decompose/decompose_test.go
git commit -m "feat(decompose): add decompose-revert and decompose-context stub"
```

---

### Task 28: `trls validate --ci` (minimal structural checks)

**Files:**
- Create: `internal/validate/validate.go`
- Create: `internal/validate/validate_test.go`
- Create: `cmd/trellis/validate.go`

- [ ] **Step 1: Write test for validation**

Create `internal/validate/validate_test.go`:

```go
package validate

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
)

func TestValidate_Clean(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"epic-01": {ID: "epic-01", Type: "epic", Title: "E", Children: []string{"task-01"}},
		"task-01": {ID: "task-01", Type: "task", Title: "T", Parent: "epic-01",
			Scope: []string{"src/**"}, DefinitionOfDone: "Done"},
	}

	result := Validate(issues)
	assert.Empty(t, result.Errors)
}

func TestValidate_OrphanParent(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Type: "task", Title: "T", Parent: "nonexistent",
			Scope: []string{"src/**"}, DefinitionOfDone: "Done"},
	}

	result := Validate(issues)
	assert.NotEmpty(t, result.Errors)
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Type: "task", Title: "T"}, // missing scope, dod
	}

	result := Validate(issues)
	assert.NotEmpty(t, result.Errors)
}
```

- [ ] **Step 2: Create validate.go**

```go
package validate

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/dag"
	"github.com/scullxbones/trellis/internal/materialize"
)

type Result struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// Validate performs E1 structural checks:
// - DFS cycle detection
// - Orphan parent references
// - Required fields for tasks (scope, definition_of_done)
func Validate(issues map[string]*materialize.Issue) Result {
	var result Result

	// Build DAG for cycle detection
	d := dag.New()
	for _, issue := range issues {
		d.AddNode(&dag.Node{
			ID:        issue.ID,
			Title:     issue.Title,
			Type:      issue.Type,
			Parent:    issue.Parent,
			Children:  issue.Children,
			BlockedBy: issue.BlockedBy,
		})
	}

	if d.HasCycle() {
		result.Errors = append(result.Errors, "dependency cycle detected in DAG")
	}

	// Orphan parent check
	for _, issue := range issues {
		if issue.Parent != "" {
			if _, ok := issues[issue.Parent]; !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("node %s references unknown parent %s", issue.ID, issue.Parent))
			}
		}
	}

	// Required fields for tasks
	for _, issue := range issues {
		if issue.Type == "task" {
			if len(issue.Scope) == 0 {
				result.Errors = append(result.Errors, fmt.Sprintf("task %s missing scope", issue.ID))
			}
			if issue.DefinitionOfDone == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("task %s missing definition_of_done", issue.ID))
			}
		}
	}

	return result
}
```

- [ ] **Step 3: Create CLI command `cmd/trellis/validate.go`**

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var repoPath string
	var ci bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate DAG structural integrity",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			if _, err := materialize.Materialize(issuesDir, true); err != nil {
				return err
			}

			index, _ := materialize.LoadIndex(issuesDir + "/state/index.json")
			issues := make(map[string]*materialize.Issue)
			for id := range index {
				issue, err := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
				if err == nil {
					issues[id] = &issue
				}
			}

			result := validate.Validate(issues)

			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(data))

			if ci && len(result.Errors) > 0 {
				return fmt.Errorf("validation failed with %d errors", len(result.Errors))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().BoolVar(&ci, "ci", false, "exit 1 on errors")
	return cmd
}
```

- [ ] **Step 4: Register, run all tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/validate/ cmd/trellis/validate.go cmd/trellis/main.go
git commit -m "feat(validate): add trls validate --ci with structural checks"
```

---

### Task 29: CLI commands for decompose-apply, decompose-revert, decompose-context

**Files:**
- Create: `cmd/trellis/decompose_apply.go`
- Create: `cmd/trellis/decompose_revert.go`
- Create: `cmd/trellis/decompose_context.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Create decompose_apply.go**

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/decompose"
	"github.com/spf13/cobra"
)

func newDecomposeApplyCmd() *cobra.Command {
	var repoPath string
	var dryRun, strict, generateIDs bool

	cmd := &cobra.Command{
		Use:   "decompose-apply [plan-file]",
		Short: "Load a plan.json into the ops log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}

			plan, err := decompose.LoadPlan(args[0])
			if err != nil {
				return err
			}

			if generateIDs {
				decompose.GenerateIDs(&plan)
			}

			// Validate — if strict, warnings also fail
			if errs := decompose.ValidatePlan(plan); len(errs) > 0 {
				if strict {
					return fmt.Errorf("strict validation failed: %v", errs)
				}
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", e)
				}
			}

			workerID, _, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}

			opsDir := repoPath + "/.issues/ops"
			result, err := decompose.ApplyPlan(plan, opsDir, workerID, dryRun)
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without writing")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail on warnings")
	cmd.Flags().BoolVar(&generateIDs, "generate-ids", false, "auto-generate node IDs")
	return cmd
}
```

- [ ] **Step 2: Create decompose_revert.go**

```go
package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/decompose"
	"github.com/spf13/cobra"
)

func newDecomposeRevertCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:   "decompose-revert [plan-file]",
		Short: "Revert a previously applied plan by cancelling all its nodes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			plan, err := decompose.LoadPlan(args[0])
			if err != nil {
				return err
			}
			workerID, _, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			opsDir := repoPath + "/.issues/ops"
			if err := decompose.RevertPlan(plan, opsDir, workerID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reverted %d nodes from plan\n", len(plan.Nodes))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	return cmd
}
```

- [ ] **Step 3: Create decompose_context.go**

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/scullxbones/trellis/internal/decompose"
	"github.com/spf13/cobra"
)

func newDecomposeContextCmd() *cobra.Command {
	var repoPath, templatePath, format, output string
	var sources []string
	var existingDAG bool

	cmd := &cobra.Command{
		Use:   "decompose-context",
		Short: "Generate decomposition context from source documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}

			result, err := decompose.DecomposeContext(sources, templatePath, format)
			if err != nil {
				return err
			}

			// If --existing-dag, load current index and attach
			if existingDAG {
				issuesDir := repoPath + "/.issues"
				indexData, err := os.ReadFile(issuesDir + "/state/index.json")
				if err == nil {
					result.ExistingDAG = json.RawMessage(indexData)
				}
			}

			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}

			if output != "" {
				if err := os.WriteFile(output, data, 0644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Context written to %s\n", output)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringSliceVar(&sources, "sources", nil, "source document paths (comma-separated)")
	cmd.Flags().StringVar(&templatePath, "template", "", "custom prompt template path")
	cmd.Flags().StringVar(&format, "format", "json", "output format")
	cmd.Flags().StringVar(&output, "output", "", "output file path")
	cmd.Flags().BoolVar(&existingDAG, "existing-dag", false, "include current DAG state in output")
	return cmd
}
```

- [ ] **Step 3: Register all decompose commands, run tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/decompose_apply.go cmd/trellis/decompose_revert.go cmd/trellis/decompose_context.go cmd/trellis/main.go
git commit -m "feat(cli): add decompose-apply, decompose-revert, and decompose-context commands"
```

---

## Chunk 8: Pre-Transition Hooks + Error Diagnostics + SKILL.md (E1-S9, E1-S10, E1-S11)

### Task 30: Pre-transition hook runner

**Files:**
- Create: `internal/hooks/runner.go`
- Create: `internal/hooks/hooks_test.go`

- [ ] **Step 1: Write test for hook execution**

Create `internal/hooks/hooks_test.go`:

```go
package hooks

import (
	"testing"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRunHook_Success(t *testing.T) {
	hook := config.HookConfig{
		Name:    "test",
		Command: "true", // always exits 0
		Required: true,
	}

	result := RunHook(hook, "/tmp", []string{"src/auth/**"})
	assert.True(t, result.Passed)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunHook_Failure(t *testing.T) {
	hook := config.HookConfig{
		Name:    "test",
		Command: "false", // always exits 1
		Required: true,
	}

	result := RunHook(hook, "/tmp", []string{"src/auth/**"})
	assert.False(t, result.Passed)
	assert.Equal(t, 1, result.ExitCode)
}

func TestRunHook_ScopeInterpolation(t *testing.T) {
	hook := config.HookConfig{
		Name:    "test",
		Command: "echo {scope}",
		Required: false,
	}

	result := RunHook(hook, "/tmp", []string{"src/auth/**"})
	assert.True(t, result.Passed)
}

func TestRunHooks_RequiredFails(t *testing.T) {
	hooks := []config.HookConfig{
		{Name: "pass", Command: "true", Required: true},
		{Name: "fail", Command: "false", Required: true},
	}

	results := RunHooks(hooks, "/tmp", []string{"src/**"})
	assert.False(t, results.AllRequiredPassed)
}

func TestRunHooks_OptionalFails(t *testing.T) {
	hooks := []config.HookConfig{
		{Name: "pass", Command: "true", Required: true},
		{Name: "fail", Command: "false", Required: false},
	}

	results := RunHooks(hooks, "/tmp", []string{"src/**"})
	assert.True(t, results.AllRequiredPassed) // optional failure doesn't block
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/brian/development/trellis && go test ./internal/hooks/ -v`
Expected: FAIL

- [ ] **Step 3: Create runner.go**

```go
package hooks

import (
	"os/exec"
	"strings"

	"github.com/scullxbones/trellis/internal/config"
)

type HookResult struct {
	Name     string
	Passed   bool
	ExitCode int
	Output   string
	Required bool
}

type HooksResult struct {
	Results            []HookResult
	AllRequiredPassed  bool
}

// RunHook executes a single verification hook.
// Exit codes: 0=pass, 1=actionable failure, other=environment error.
func RunHook(hook config.HookConfig, workDir string, scope []string) HookResult {
	cmdStr := interpolateScope(hook.Command, scope)

	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 127 // command not found or other error
		}
	}

	return HookResult{
		Name:     hook.Name,
		Passed:   exitCode == 0,
		ExitCode: exitCode,
		Output:   string(output),
		Required: hook.Required,
	}
}

// RunHooks executes all configured hooks and aggregates results.
func RunHooks(hooks []config.HookConfig, workDir string, scope []string) HooksResult {
	result := HooksResult{AllRequiredPassed: true}

	for _, hook := range hooks {
		hr := RunHook(hook, workDir, scope)
		result.Results = append(result.Results, hr)
		if hr.Required && !hr.Passed {
			result.AllRequiredPassed = false
		}
	}

	return result
}

func interpolateScope(command string, scope []string) string {
	scopeStr := strings.Join(scope, " ")
	return strings.ReplaceAll(command, "{scope}", scopeStr)
}

// RunPreTransitionHooks runs hooks with strict phase separation:
// - Verification commands execute in the CODE worktree (codeDir)
// - Results are recorded via ops in the OPS worktree (opsDir)
// This separation is critical: hooks must never run in the ops worktree.
func RunPreTransitionHooks(hooks []config.HookConfig, codeDir string, opsDir string, scope []string, issueID string, workerID string) (HooksResult, error) {
	// Phase 1: Run all hooks in the code worktree
	result := RunHooks(hooks, codeDir, scope)

	// Phase 2: Record results as note ops in the ops worktree
	logPath := filepath.Join(opsDir, workerID+".log")
	for _, hr := range result.Results {
		status := "passed"
		if !hr.Passed {
			status = fmt.Sprintf("failed (exit %d)", hr.ExitCode)
		}
		noteMsg := fmt.Sprintf("hook %q %s: %s", hr.Name, status, strings.TrimSpace(hr.Output))
		// Record as note op (importing ops package)
		// ops.AppendOp(logPath, ops.Op{Type: ops.OpNote, TargetID: issueID,
		//   Timestamp: time.Now().Unix(), WorkerID: workerID,
		//   Payload: ops.Payload{Msg: noteMsg}})
	}

	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/brian/development/trellis && go test ./internal/hooks/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hooks/
git commit -m "feat(hooks): add pre-transition hook runner with scope interpolation and phase separation"
```

---

### Task 30b: Wire hooks into transition command

**Files:**
- Modify: `cmd/trellis/transition.go`

- [ ] **Step 1: Add hook execution before transition op is written**

In `newTransitionCmd`'s `RunE`, after resolving the worker and before appending the transition op, add:

```go
// Load config and run pre-transition hooks
cfg, cfgErr := config.LoadConfig(repoPath + "/.issues/config.json")
if cfgErr == nil && len(cfg.Hooks) > 0 {
	// Load issue for scope
	issuesDir := repoPath + "/.issues"
	materialize.Materialize(issuesDir, true)
	issue, loadErr := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, issueID))
	if loadErr == nil {
		hookResult, _ := hooks.RunPreTransitionHooks(cfg.Hooks, repoPath, issuesDir+"/ops", issue.Scope, issueID, workerID)
		if !hookResult.AllRequiredPassed {
			return fmt.Errorf("required pre-transition hook failed — transition blocked")
		}
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd /home/brian/development/trellis && go test ./... -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/trellis/transition.go
git commit -m "feat(cli): wire pre-transition hooks into transition command"
```

---

### Task 31: Structured error diagnostics

**Files:**
- Create: `internal/errors/errors.go`
- Modify: `cmd/trellis/main.go` (wire --debug flag)

- [ ] **Step 1: Write test for structured errors**

Create `internal/errors/errors_test.go`:

```go
package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructuredError(t *testing.T) {
	err := New("claim failed", "issue task-01 is already claimed by worker-b2", "run 'trls ready' to find available tasks")
	assert.Contains(t, err.Error(), "claim failed")
	assert.Contains(t, err.Hint, "trls ready")
}

func TestOpsWorktreeNotFoundError(t *testing.T) {
	err := OpsWorktreeNotFound()
	assert.Contains(t, err.Error(), "dual-branch mode not active")
	assert.Contains(t, err.Hint, "trls init")
}
```

- [ ] **Step 2: Create errors.go**

```go
package errors

import "fmt"

// TrellisError is a structured error with context and hints.
type TrellisError struct {
	Message string `json:"message"`
	State   string `json:"relevant_state,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

func (e *TrellisError) Error() string {
	return fmt.Sprintf("%s: %s", e.Message, e.State)
}

func New(message, state, hint string) *TrellisError {
	return &TrellisError{Message: message, State: state, Hint: hint}
}

// OpsWorktreeNotFound returns an informative stub error for E1 single-branch mode.
func OpsWorktreeNotFound() *TrellisError {
	return &TrellisError{
		Message: "ops branch not found",
		State:   "dual-branch mode not active",
		Hint:    "run 'trls init' in a repo with branch protection, or after E2 is complete",
	}
}

// OpsWorktreeDesync returns a stub error for E1.
func OpsWorktreeDesync() *TrellisError {
	return &TrellisError{
		Message: "ops worktree desync",
		State:   "dual-branch mode not active",
		Hint:    "run 'trls init' in a repo with branch protection, or after E2 is complete",
	}
}

// MaterializationFailed returns an error for corrupt log lines.
func MaterializationFailed(logFile string, lineNum int, detail string) *TrellisError {
	return &TrellisError{
		Message: "materialization failed",
		State:   fmt.Sprintf("corrupt line %d in %s: %s", lineNum, logFile, detail),
		Hint:    "the corrupt line will be skipped; run 'trls materialize' to rebuild state",
	}
}
```

- [ ] **Step 3: Add --debug dump function**

Add to `internal/errors/errors.go`:

```go
// DebugDump collects diagnostic info when --debug flag is set.
// Dumps: materialized issue, raw log entries, git status, checkpoint state.
func DebugDump(issuesDir string, issueID string) string {
	var b strings.Builder

	b.WriteString("=== DEBUG DUMP ===\n\n")

	// Materialized issue
	issuePath := filepath.Join(issuesDir, "state", "issues", issueID+".json")
	if data, err := os.ReadFile(issuePath); err == nil {
		fmt.Fprintf(&b, "--- Materialized Issue (%s) ---\n%s\n\n", issueID, string(data))
	} else {
		fmt.Fprintf(&b, "--- Materialized Issue: not found (%v) ---\n\n", err)
	}

	// Raw log entries for this issue
	opsDir := filepath.Join(issuesDir, "ops")
	if entries, err := os.ReadDir(opsDir); err == nil {
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".log") {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(opsDir, entry.Name())); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.Contains(line, issueID) {
						fmt.Fprintf(&b, "--- %s ---\n%s\n", entry.Name(), line)
					}
				}
			}
		}
	}

	// Checkpoint state
	cpPath := filepath.Join(issuesDir, "state", "checkpoint.json")
	if data, err := os.ReadFile(cpPath); err == nil {
		fmt.Fprintf(&b, "\n--- Checkpoint ---\n%s\n", string(data))
	}

	// Git status
	cmd := exec.Command("git", "status", "--short")
	if out, err := cmd.Output(); err == nil {
		fmt.Fprintf(&b, "\n--- Git Status ---\n%s\n", string(out))
	}

	return b.String()
}
```

- [ ] **Step 4: Wire --debug into root command error handling**

In `cmd/trellis/main.go`, update the root command's `PersistentPostRunE` or error handling to check `--debug` and call `errors.DebugDump` when an error occurs with the flag set.

- [ ] **Step 5: Run tests, commit**

Run: `cd /home/brian/development/trellis && go test ./internal/errors/ -v`
Expected: PASS

```bash
git add internal/errors/
git commit -m "feat(errors): add structured error format, --debug dump, and ops worktree stubs"
```

---

### Task 32: SKILL.md — AI worker interface document

**Files:**
- Create: `docs/SKILL.md`

- [ ] **Step 1: Write SKILL.md**

This is the complete AI worker interface document per E1-S11. It must include:
- Setup (2 commands: `trls version`, `trls worker-init --check`)
- Work loop (7 steps)
- Error recovery (blocked, wrong task, PR rejected)
- Rules and constraints
- NO reference to `trls confirm` (doesn't exist until E3-S8)
- Inferred nodes cannot be claimed (error message only)

```markdown
# Trellis Worker Skill

## Setup

Before starting work, verify your environment:

1. `trls version` — confirm CLI is installed and accessible
2. `trls worker-init --check` — confirm worker identity is configured

If worker-init fails, run `trls worker-init` to generate a new identity.

## Work Loop

Repeat these steps for each work item:

1. **Sync and materialize:** `trls materialize`
2. **Find ready work:** `trls ready --format=json`
   - Pick the highest-priority task from the list
   - If no tasks are ready, wait for blockers to complete
3. **Get context:** `trls render-context --issue <id> --format=agent`
   - Read the full context before starting work
4. **Claim the task:** `trls claim --issue <id> --ttl 60`
   - If claim fails with "inferred" error: this node has not been confirmed by a human. Skip it and pick another task.
5. **Do the work:**
   - Follow the scope, definition of done, and acceptance criteria from the context
   - Send heartbeats periodically: `trls heartbeat --issue <id>`
   - Add notes for discoveries: `trls note --issue <id> --msg "..."`
6. **Complete the task:** `trls transition --issue <id> --to done --outcome "description of what was done"`
7. **Repeat from step 1**

## Error Recovery

### Blocked — cannot complete work
```
trls transition --issue <id> --to blocked
trls note --issue <id> --msg "Blocked because: <reason>"
```
Return to step 1 — pick a different task.

### Wrong task — claimed by mistake
```
trls reopen --issue <id>
```
Return to step 1.

### PR rejected — work needs revision
```
trls reopen --issue <id>
trls note --issue <id> --msg "PR rejected: <reason>"
trls claim --issue <id> --ttl 60
```
Rework and re-transition to done.

### Merge not detected automatically
If your task is stuck at `done` and the code is on main:
```
trls merged --issue <id>
```
Note: In single-branch mode this is automatic. This command exists for dual-branch mode (E2).

## Rules

- **One task at a time.** Complete or release your current claim before taking another.
- **Heartbeat every significant operation.** Prevents your claim from expiring.
- **Stay within scope.** Only modify files listed in the task's scope globs.
- **Record decisions.** Use `trls decision --issue <id> --topic <topic> --choice <choice> --rationale <why>` for architectural choices that affect other tasks.
- **Never skip tests.** Run the project's test suite before transitioning to done.
- **Inferred nodes cannot be claimed.** If you see "requires confirmation" on a ready task, skip it — a human must confirm it first.
```

- [ ] **Step 2: Commit**

```bash
git add docs/SKILL.md
git commit -m "docs: add SKILL.md AI worker interface"
```

---

### Task 33: plan-post-bootstrap.json — E2/E3/E4 plan for dogfood ceremony

**Files:**
- Create: `docs/plan-post-bootstrap.json`

- [ ] **Step 1: Generate plan-post-bootstrap.json**

This is a first-class deliverable of E1-S11. It must contain E2, E3, and E4 as epics with all stories, acceptance criteria, source citations, and scope globs. The plan must be valid according to `trls decompose-apply --dry-run`.

The implementing agent must:
1. Read `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` for all E2/E3/E4 story details
2. Read `docs/architecture.md` for technical specifications
3. Read `docs/trellis-prd.md` for requirements
4. Generate a complete plan.json with:
   - 3 epics (E2, E3, E4)
   - All stories from the spec as tasks under their respective epics
   - `blocked_by` edges matching the spec's dependency graph
   - `scope` globs for each task (inferred from deliverables)
   - `definition_of_done` for each task
   - `acceptance` criteria for each task
   - `source_citation` referencing PRD and architecture doc sections
   - `priority` fields
   - `estimated_complexity` fields

This task is comparable in effort to writing SKILL.md itself — it requires careful reading of the spec and architecture doc.

- [ ] **Step 2: Validate the plan**

Run: `cd /home/brian/development/trellis && trls decompose-apply docs/plan-post-bootstrap.json --dry-run`
Expected: Validation passes, shows nodes that would be created

- [ ] **Step 3: Commit**

```bash
git add docs/plan-post-bootstrap.json
git commit -m "feat: add plan-post-bootstrap.json for dogfood ceremony (E2/E3/E4)"
```

---

### Task 34: Dogfood ceremony — end-to-end verification

This task executes the dogfood ceremony described in the spec to verify E1 is complete.

- [ ] **Step 1: Initialize Trellis in the trellis repo**

```bash
cd /home/brian/development/trellis
trls init
```

- [ ] **Step 2: Register source documents as local filesystem sources**

```bash
trls decompose-context --sources docs/trellis-prd.md,docs/architecture.md --format json --output /tmp/context.json
```

- [ ] **Step 3: Dry-run the plan**

```bash
trls decompose-apply docs/plan-post-bootstrap.json --dry-run
```
Expected: Validation passes

- [ ] **Step 4: Apply the plan**

```bash
trls decompose-apply docs/plan-post-bootstrap.json
```
Expected: All nodes created

- [ ] **Step 5: Validate DAG integrity**

```bash
trls validate --ci
```
Expected: Exit 0, no errors

- [ ] **Step 6: Confirm tasks are available**

```bash
trls ready --format=json
```
Expected: JSON array with ready tasks

- [ ] **Step 7: Claim and complete one task**

```bash
trls claim --issue <first-ready-task-id> --ttl 60
trls heartbeat --issue <first-ready-task-id>
trls transition --issue <first-ready-task-id> --to done --outcome "Dogfood ceremony verification"
```

- [ ] **Step 8: Verify final state**

```bash
trls materialize
trls ready --format=json
```
Expected: The completed task no longer appears in ready queue

- [ ] **Step 9: Commit ceremony results**

```bash
git add .issues/
git commit -m "ceremony: complete E1 dogfood transition — Trellis managing itself"
```

---

## Success Criteria

- [ ] All 10 op types defined and parseable (E1-S1)
- [ ] Per-worker log files with append/read/validation (E1-S1)
- [ ] Rate limiters for heartbeats and creates (E1-S1)
- [ ] `trls materialize` standalone CLI command (E1-S2)
- [ ] Incremental materialization via byte offsets (E1-S2)
- [ ] Single-branch auto-merge (done→merged) (E1-S2)
- [ ] Bottom-up rollup (story/epic auto-promote) (E1-S2)
- [ ] 4-rule ready gate with priority sort (E1-S3)
- [ ] Inferred nodes blocked from claiming (E1-S3)
- [ ] Claim race resolution (timestamp + lexicographic) (E1-S4)
- [ ] Scope overlap advisory with auto-notes (E1-S4)
- [ ] 7-layer context assembly with truncation (E1-S5)
- [ ] `trls render-context` with --format=agent and --format=human (E1-S5)
- [ ] All status transitions including reverse (E1-S6)
- [ ] `trls merged` stub command (E1-S6)
- [ ] `trls init` single-branch mode (E1-S7)
- [ ] `trls version` and `trls worker-init` (E1-S7)
- [ ] `trls decompose-apply` with validation, dry-run, idempotency (E1-S8)
- [ ] `trls decompose-context` stub with local filesystem (E1-S8)
- [ ] `trls validate --ci` structural checks (E1-S8)
- [ ] Pre-transition hooks with scope interpolation (E1-S9)
- [ ] Structured error format with ops-worktree stubs (E1-S10)
- [ ] SKILL.md complete AI worker interface (E1-S11)
- [ ] plan-post-bootstrap.json with E2/E3/E4 (E1-S11)
- [ ] Dogfood ceremony passes (E1 exit criterion)
- [ ] Property tests for ops, materialization, and DAG invariants
- [ ] All tests pass: `go test ./...`
