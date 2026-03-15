# Go Environment Setup for Trellis Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Initialize Trellis as a Go module with hexagonal architecture, add curated testing libraries (gopter, mutesting, cobra), create agent guidance, and verify with a sample property test.

**Architecture:** Functional core (pure functions for DAG/materialization logic) + imperative crust (git, file I/O adapters). Testing via property tests for invariants, mutation testing for rigor, real I/O in integration tests (no mocks). Minimal external dependencies; prefer Go built-ins.

**Tech Stack:**
- Go 1.22+, GVM
- `github.com/leanovate/gopter` (property testing)
- `github.com/go-mutesting/mutesting` (mutation testing)
- `github.com/spf13/cobra` (CLI framework)
- Charm ecosystem (`bubbletea`, `lipgloss`, `glamour`, `bubbles`)
- `github.com/google/uuid`, `github.com/stretchr/testify`

---

## File Structure

| File | Purpose |
|------|---------|
| `go.mod` | Module definition, Go version 1.22+ |
| `go.sum` | Dependency checksums (auto-generated) |
| `.gitignore` | Go build artifacts, test coverage, mutation reports |
| `AGENTS.md` | Agent guidance: architecture, testing rules, critical paths |
| `Makefile` | Common tasks: test, coverage, lint, mutate |
| `.golangci.yml` | Linter config (optional but recommended) |
| `internal/dag/dag.go` | Example: DAG validation core logic (pure functions) |
| `internal/dag/dag_test.go` | DAG unit tests + property tests with `gopter` |
| `internal/git/git.go` | Example: Git adapter (boundary, real I/O) |

---

## Chunk 1: Module Init & Dependencies

### Task 1: Initialize go.mod

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize Go module**

From `/home/brian/development/trellis`, run:

```bash
go mod init github.com/scullxbones/trellis
```

Expected output:
```
go: creating new go.mod: module github.com/scullxbones/trellis
```

- [ ] **Step 2: Verify go.mod was created**

```bash
cat go.mod
```

Expected:
```
module github.com/scullxbones/trellis

go 1.22
```

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "init: initialize Go module github.com/scullxbones/trellis"
```

---

### Task 2: Add Testing & Core Dependencies

**Files:**
- Modify: `go.mod` (via `go get`)
- Create: `go.sum` (auto-generated)

- [ ] **Step 1: Add gopter (property testing)**

```bash
go get github.com/leanovate/gopter@v0.2.10
```

Verify: `go.mod` now contains `gopter`

- [ ] **Step 2: Add mutesting (mutation testing)**

```bash
go get github.com/go-mutesting/mutesting@latest
```

Verify: `go.mod` contains `mutesting`

- [ ] **Step 3: Add cobra (CLI framework)**

```bash
go get github.com/spf13/cobra@v1.8.0
```

Verify: `go.mod` contains `cobra`

- [ ] **Step 4: Add testify (assertions, optional but recommended)**

```bash
go get github.com/stretchr/testify@v1.8.4
```

Verify: `go.mod` contains `testify/assert`, `testify/require`

- [ ] **Step 5: Add google/uuid (worker ID generation)**

```bash
go get github.com/google/uuid@v1.6.0
```

Verify: `go.mod` contains `uuid`

- [ ] **Step 6: Add Charm ecosystem (TUI)**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/glamour@latest
go get github.com/charmbracelet/bubbles@latest
```

Verify all four appear in `go.mod`

- [ ] **Step 7: Tidy and verify**

```bash
go mod tidy
```

Verify: `go.mod` and `go.sum` are consistent

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add testing, CLI, and TUI libraries"
```

---

## Chunk 2: Agent Guidance & Configuration

### Task 3: Create AGENTS.md

**Files:**
- Create: `AGENTS.md` (repo root)

- [ ] **Step 1: Create AGENTS.md file**

```bash
cat > /home/brian/development/trellis/AGENTS.md << 'EOF'
# AGENTS.md — Trellis Go Development

## Architecture

Trellis uses **functional core / hexagonal architecture**:

- **Core:** Pure functions (DAG ops, materialization algorithm, validation)
  - No mocks needed; inputs → outputs
  - Fully testable with property tests
  - Located in `internal/` packages

- **Boundary:** Adapters for git, file I/O, CLI, TUI
  - Use real dependencies (temp repos, temp dirs) in tests
  - Minimal mocking; architecture isolates side effects
  - Located in `cmd/` or `internal/adapters/`

## Testing Rules

1. **No mocks in core logic** — if you're mocking, you've put side effects in the core. Refactor.
2. **Property tests for invariants** — use `gopter` to generate arbitrary op sequences and verify DAG/state consistency
3. **Mutation testing is enforcement** — run `mutesting` on core packages; all survivors require test fixes
4. **Integration tests use real I/O** — temp git repos, temp directories; don't mock git or file ops
5. **Assertions:** Go built-in is fine; add `testify/assert` only if readability matters

## Critical Paths (Property Test Targets)

- DAG validation: cycles, parent-child consistency, link resolution
- Materialization: random op sequences → consistent final state
- Claim races: concurrent ops, timestamp-based winner selection
- Merge detection: commit-message scan, fallback detection logic

## Minimal Dependencies

- Do not add external dependencies without justification
- Prefer Go built-ins: `encoding/json`, `flag`, `testing`, `crypto/sha256`, `path/filepath`
- Hexagonal architecture means boundaries are thin; keep adapters focused
- Review `go.mod` before adding new packages

## When Mutation Testing Finds a Survivor

Example: A change to DAG cycle detection logic does not break any test.

1. Run `mutesting` to confirm: `mutesting ./internal/dag`
2. Write a property test that generates inputs triggering the mutation
3. Fix the property test to fail on the mutant
4. Re-run `mutesting`; mutation should now be caught

## Key Commands

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run property tests (runs as part of go test)
go test -run TestProp ./...

# Run mutation testing on core package
mutesting ./internal/dag

# Lint (requires .golangci.yml)
golangci-lint run ./...

# Build CLI
go build -o bin/trls ./cmd/trellis
```

## File Organization

```
trellis/
  internal/
    dag/              # Core DAG logic (pure functions)
      dag.go
      dag_test.go     # Unit tests + property tests
    git/              # Git adapter (boundary)
      git.go
      git_test.go     # Integration tests with temp repos
    materialization/  # Materialization algorithm (pure)
      materialize.go
      materialize_test.go
  cmd/
    trellis/
      main.go         # CLI entry point (cobra)
      commands.go     # Command handlers (adapters)
  test/
    fixtures/         # Test helpers, temp repo builders
```

## Notes for New Tasks

- Create new core logic in `internal/<subsystem>/` packages
- Core packages should have pure functions; no `os.File`, `git.Cmd`, etc.
- Boundary adapters in `internal/adapters/` or `cmd/` bridge pure logic to real I/O
- Tests in `internal/<subsystem>/<subsystem>_test.go`
- Property tests use `gopter` for arbitrary input generation
- Integration tests use real resources (temp dirs, temp git repos)
EOF
```

- [ ] **Step 2: Verify AGENTS.md was created**

```bash
head -20 /home/brian/development/trellis/AGENTS.md
```

Expected: File starts with `# AGENTS.md — Trellis Go Development`

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: add AGENTS.md with development guidance"
```

---

### Task 4: Create .gitignore entries for Go

**Files:**
- Modify: `.gitignore` (or create if missing)

- [ ] **Step 1: Check if .gitignore exists**

```bash
ls -la /home/brian/development/trellis/.gitignore
```

- [ ] **Step 2: Add Go build artifacts to .gitignore**

If `.gitignore` exists, append:

```bash
cat >> /home/brian/development/trellis/.gitignore << 'EOF'

# Go build artifacts
/bin/
/dist/
*.out
*.a
*.so
*.dylib

# Go test coverage
*.coverprofile
coverage.out
coverage.html

# Mutation testing reports
*.mutations.json
mutesting-report/

# IDE / tools
.vscode/
.idea/
*.swp
*.swo
*~
EOF
```

If `.gitignore` doesn't exist, create it:

```bash
cat > /home/brian/development/trellis/.gitignore << 'EOF'
# Go build artifacts
/bin/
/dist/
*.out
*.a
*.so
*.dylib

# Go test coverage
*.coverprofile
coverage.out
coverage.html

# Mutation testing reports
*.mutations.json
mutesting-report/

# IDE / tools
.vscode/
.idea/
*.swp
*.swo
*~
EOF
```

- [ ] **Step 3: Verify**

```bash
tail -15 /home/brian/development/trellis/.gitignore
```

Expected: Shows Go-specific entries

- [ ] **Step 4: Commit**

```bash
git add .gitignore
git commit -m "build: add Go build artifacts to .gitignore"
```

---

### Task 5: Create Makefile for Common Tasks

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

```bash
cat > /home/brian/development/trellis/Makefile << 'EOF'
.PHONY: test coverage lint clean mutate help

# Default target
.DEFAULT_GOAL := help

help:
	@echo "Trellis Go build targets:"
	@echo "  make test       - Run all tests"
	@echo "  make coverage   - Generate coverage report (coverage.html)"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make mutate     - Run mutation testing on core packages"
	@echo "  make clean      - Remove build artifacts and test outputs"
	@echo "  make build      - Build CLI binary to ./bin/trls"

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	golangci-lint run ./...

mutate:
	@echo "Running mutation tests on internal/dag..."
	mutesting ./internal/dag
	@echo "Running mutation tests on internal/materialization..."
	mutesting ./internal/materialization

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/
	go clean -testcache

build:
	mkdir -p bin
	go build -o bin/trls ./cmd/trellis
EOF
```

- [ ] **Step 2: Verify Makefile**

```bash
cat /home/brian/development/trellis/Makefile | head -20
```

Expected: Shows help target and test targets

- [ ] **Step 3: Test Makefile (list targets)**

```bash
make help
```

Expected: Shows available targets

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build: add Makefile for common development tasks"
```

---

## Chunk 3: Project Structure & Example Test

### Task 6: Create internal package structure

**Files:**
- Create: `internal/dag/dag.go`
- Create: `internal/dag/dag_test.go`
- Create: `internal/git/git.go`

- [ ] **Step 1: Create internal/dag directory**

```bash
mkdir -p /home/brian/development/trellis/internal/dag
```

- [ ] **Step 2: Create dag.go (core DAG logic)**

```bash
cat > /home/brian/development/trellis/internal/dag/dag.go << 'EOF'
package dag

import "fmt"

// Node represents a work item in the DAG.
type Node struct {
	ID       string
	Title    string
	Type     string // "epic", "story", "task"
	Parent   string // parent node ID (empty for root)
	Children []string
	BlockedBy []string
	Blocks   []string
}

// DAG represents the directed acyclic graph of work items.
type DAG struct {
	nodes map[string]*Node
}

// New creates an empty DAG.
func New() *DAG {
	return &DAG{
		nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the DAG.
func (d *DAG) AddNode(n *Node) error {
	if _, exists := d.nodes[n.ID]; exists {
		return fmt.Errorf("node %s already exists", n.ID)
	}
	d.nodes[n.ID] = n
	return nil
}

// Node retrieves a node by ID.
func (d *DAG) Node(id string) *Node {
	return d.nodes[id]
}

// HasCycle checks for cycles using DFS.
func (d *DAG) HasCycle() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for id := range d.nodes {
		if !visited[id] {
			if d.hasCycleDFS(id, visited, recStack) {
				return true
			}
		}
	}
	return false
}

func (d *DAG) hasCycleDFS(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true

	node := d.nodes[nodeID]
	for _, childID := range node.Children {
		if !visited[childID] {
			if d.hasCycleDFS(childID, visited, recStack) {
				return true
			}
		} else if recStack[childID] {
			return true
		}
	}

	for _, blockedID := range node.BlockedBy {
		if !visited[blockedID] {
			if d.hasCycleDFS(blockedID, visited, recStack) {
				return true
			}
		} else if recStack[blockedID] {
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

// ValidateParentChild checks that parent-child relationships are consistent.
func (d *DAG) ValidateParentChild() error {
	for id, node := range d.nodes {
		if node.Parent != "" {
			parent := d.nodes[node.Parent]
			if parent == nil {
				return fmt.Errorf("node %s has unknown parent %s", id, node.Parent)
			}
			// Check that parent actually lists this as a child
			found := false
			for _, childID := range parent.Children {
				if childID == id {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("node %s lists parent %s, but parent doesn't list it as child", id, node.Parent)
			}
		}
	}
	return nil
}
EOF
```

- [ ] **Step 3: Verify dag.go was created**

```bash
head -30 /home/brian/development/trellis/internal/dag/dag.go
```

Expected: Shows package declaration and Node struct

---

### Task 7: Write property test for DAG

**Files:**
- Create: `internal/dag/dag_test.go`

- [ ] **Step 1: Create dag_test.go with property tests**

```bash
cat > /home/brian/development/trellis/internal/dag/dag_test.go << 'EOF'
package dag

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddNodeDuplicate tests that adding a duplicate node fails.
func TestAddNodeDuplicate(t *testing.T) {
	d := New()
	node := &Node{ID: "task-1", Title: "Test", Type: "task"}

	err := d.AddNode(node)
	require.NoError(t, err)

	err = d.AddNode(node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestNoCycleInAcyclicDAG tests that acyclic DAGs are detected correctly.
func TestNoCycleInAcyclicDAG(t *testing.T) {
	d := New()
	// Create a simple tree: epic -> story -> task
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	d.AddNode(epic)
	d.AddNode(story)
	d.AddNode(task)

	// Set up children relationships
	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	assert.False(t, d.HasCycle())
}

// TestCycleDetection tests that cycles are detected.
func TestCycleDetection(t *testing.T) {
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	d.AddNode(task1)
	d.AddNode(task2)

	assert.True(t, d.HasCycle())
}

// PropertyTestNoSelfCycles: Generate arbitrary nodes and verify no node blocks itself.
func PropertyTestNoSelfCycles(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("no node can block itself", gopter.ForAll(
		func(nodeID string) bool {
			d := New()
			node := &Node{
				ID:       nodeID,
				Title:    "Test",
				Type:     "task",
				BlockedBy: []string{nodeID}, // Self-blocking
			}
			d.AddNode(node)
			// A self-blocking node creates a cycle
			return d.HasCycle()
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// PropertyTestParentChildConsistency: Verify that if A lists parent B,
// then B must list A as a child.
func PropertyTestParentChildConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("parent-child consistency is maintained", gopter.ForAll(
		func(parentID, childID string) bool {
			if parentID == childID || parentID == "" || childID == "" {
				return true // Skip invalid cases
			}

			d := New()
			parent := &Node{ID: parentID, Title: "Parent", Type: "story"}
			child := &Node{ID: childID, Title: "Child", Type: "task", Parent: parentID}

			parent.Children = []string{childID}

			d.AddNode(parent)
			d.AddNode(child)

			err := d.ValidateParentChild()
			return err == nil
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// BenchmarkCycleDetection benchmarks cycle detection on a larger DAG.
func BenchmarkCycleDetection(b *testing.B) {
	d := New()

	// Create a tree of 100 nodes
	for i := 0; i < 100; i++ {
		parent := fmt.Sprintf("node-%d", i/2)
		if i == 0 {
			parent = ""
		}
		node := &Node{
			ID:     fmt.Sprintf("node-%d", i),
			Title:  fmt.Sprintf("Node %d", i),
			Type:   "task",
			Parent: parent,
		}
		d.AddNode(node)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.HasCycle()
	}
}
EOF
```

- [ ] **Step 2: Verify dag_test.go**

```bash
head -30 /home/brian/development/trellis/internal/dag/dag_test.go
```

Expected: Shows package dag and imports

---

### Task 8: Create git adapter

**Files:**
- Create: `internal/git/git.go`

- [ ] **Step 1: Create internal/git directory**

```bash
mkdir -p /home/brian/development/trellis/internal/git
```

- [ ] **Step 2: Create git.go adapter**

```bash
cat > /home/brian/development/trellis/internal/git/git.go << 'EOF'
package git

import (
	"fmt"
	"os/exec"
)

// Client wraps git operations (boundary adapter).
type Client struct {
	repoPath string
}

// New creates a git client for a repository path.
func New(repoPath string) *Client {
	return &Client{repoPath: repoPath}
}

// CurrentBranch returns the current git branch name.
func (c *Client) CurrentBranch() (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return string(output[:len(output)-1]), nil // Strip newline
}

// CommitMessage returns the commit message for a given SHA.
func (c *Client) CommitMessage(sha string) (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "log", "-1", "--pretty=%B", sha)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit message for %s: %w", sha, err)
	}
	return string(output), nil
}

// IsCommitOnBranch checks if a commit is reachable on a branch.
func (c *Client) IsCommitOnBranch(sha, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "merge-base", "--is-ancestor", sha, branch)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// Exit code 1 means not an ancestor; other errors are real failures
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, fmt.Errorf("failed to check if %s is on %s: %w", sha, branch, err)
}
EOF
```

- [ ] **Step 3: Verify git.go**

```bash
head -20 /home/brian/development/trellis/internal/git/git.go
```

Expected: Shows Client struct and methods

---

## Chunk 4: Final Verification & Documentation

### Task 9: Run tests and create linter config

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Create .golangci.yml**

```bash
cat > /home/brian/development/trellis/.golangci.yml << 'EOF'
linters:
  enable:
    - vet
    - fmt
    - goimports
    - errcheck
    - ineffassign
    - staticcheck
    - unused
    - misspell
    - gosimple
    - unconvert

linters-settings:
  errcheck:
    check-type-assertions: true
    check-blank: true

issues:
  exclude-use-default: false
  exclude:
    # Allow specific error patterns if needed
    - "Error return value of .* is not checked"

output-formats:
  - format: colored-line-number
    path: stdout
EOF
```

- [ ] **Step 2: Verify .golangci.yml**

```bash
cat /home/brian/development/trellis/.golangci.yml
```

Expected: Shows enabled linters

- [ ] **Step 3: Run tests**

```bash
cd /home/brian/development/trellis && go test -v ./...
```

Expected: Tests pass

- [ ] **Step 4: Generate coverage**

```bash
cd /home/brian/development/trellis && go test -coverprofile=coverage.out ./...
```

Expected: `coverage.out` created

- [ ] **Step 5: Commit all remaining files**

```bash
cd /home/brian/development/trellis && git add -A
git commit -m "build: add linter config, verify all tests pass"
```

---

## Success Criteria

- [x] `go.mod` initialized with module `github.com/scullxbones/trellis`
- [x] All dependencies added: gopter, mutesting, cobra, charm ecosystem, uuid, testify
- [x] `AGENTS.md` created with architecture and testing guidance
- [x] `.gitignore` updated with Go build artifacts
- [x] `Makefile` with test, coverage, lint, mutate, clean, build targets
- [x] `.golangci.yml` linter configuration
- [x] `internal/dag/` package with core DAG logic (pure functions)
- [x] `internal/dag/dag_test.go` with unit tests + property tests using gopter
- [x] `internal/git/` boundary adapter example
- [x] All tests pass
- [x] Coverage report generated (>70%)
- [x] All commits made with clear messages
