# Trellis Go Environment Design

**Date:** 2026-03-14
**Scope:** GVM setup, library stack, testing infrastructure, agent guidance

---

## Overview

Set up a Go development environment for Trellis using GVM with a carefully selected library stack optimized for a **functional core / hexagonal architecture**. Emphasis on high-rigor mutation testing, property-based invariant testing, and minimal mocking through architectural discipline.

---

## Library Stack

### Test Framework & Assertions
- **Go built-in `testing` package** — no external dependency
- **`github.com/stretchr/testify/assert` & `require`** (optional) — readable assertions, fail-fast on integration tests

### Property-Based Testing
- **`github.com/leanovate/gopter`** — QuickCheck-style invariant testing for arbitrary op sequences, DAG consistency, state transitions, concurrent claim races

### Mutation Testing
- **`github.com/go-mutesting/mutesting`** — native Go mutation framework for code mutation + test survivorship tracking
- **Supplement with `gopter` property tests** — inherently catch mutations through random input generation

### Code Coverage
- **`go tool cover` (built-in)** — coverage reports
- **`github.com/axw/gocov`** (optional) — extended per-function analysis

### CLI Framework
- **`github.com/spf13/cobra`** — feature-rich subcommand structure, nested groups, auto-help

### TUI
- **Charm ecosystem** (already chosen): `bubbletea`, `lipgloss`, `glamour`, `bubbles`

### Utilities
- **`github.com/google/uuid`** — worker ID generation
- **`encoding/json` (built-in)** — JSONL parsing
- **`github.com/golang/mock`** (sparse, boundary adapters only) — idiomatic Go mocking

---

## Testing Strategy

**Principle:** Architecture carries the testing load. Functional core functions are pure (no mocks needed). Hexagonal boundaries use real I/O (git, files) instead of mocks.

### Pure Function Tests (Unit, Fast)
- DAG validation, op parsing, materialization algorithm, ready-task computation
- No mocks, no external I/O
- Mutation testing target

### Boundary Tests (Integration, Real I/O)
- Git operations: real temp repos
- File I/O: temp `.trellis/` directories
- Concurrent claim races: real file locking, timestamp resolution
- No mocks; architecture isolates side effects

### Property Tests (Both Layers)
- Generate random op sequences → verify DAG invariants (no cycles, parent-child consistency)
- Arbitrary materialization scenarios → verify state correctness (ready tasks, status rollups)
- Concurrent claim patterns → verify timestamp-based resolution is deterministic

### Mutation Testing
1. `mutesting` mutates core logic
2. Run test suite; count survivors
3. Property tests catch mutations implicitly through arbitrary input generation
4. High rigor: all survivors require test fixes

---

## Priorities (Ranked)

1. **DAG Consistency** — property tests on cycle detection, parent-child relationships, link validation
2. **Materialization Correctness** — property tests on op sequences, state reconstruction, rollup logic
3. **Concurrent Claim Races** — integration tests with real file locking, timestamp resolution
4. **Merge Detection Accuracy** — traditional unit tests on commit-message scan logic

---

## AGENTS.md Guidance

Minimal, focused constraints for agents working on Trellis:

```markdown
# AGENTS.md — Trellis Go Development

## Architecture

Trellis uses **functional core / hexagonal architecture**:
- **Core:** Pure functions (DAG ops, materialization algorithm, validation)
  - No mocks needed; inputs → outputs
  - Fully testable with property tests
- **Boundary:** Adapters for git, file I/O, CLI, TUI
  - Use real dependencies (temp repos, temp dirs) in tests
  - Minimal mocking; architecture isolates side effects

## Testing Rules

1. **No mocks in core logic** — if you're mocking, you've put side effects in the core
2. **Property tests for invariants** — use `gopter` to generate arbitrary op sequences and verify DAG/state consistency
3. **Mutation testing is enforcement** — all survivors require test fixes (use `mutesting`)
4. **Integration tests use real I/O** — temp git repos, temp directories; don't mock git or file ops
5. **Assertions:** Go built-in is fine; add `testify` only if readability matters

## Critical Paths (Property Test Targets)

- DAG validation: cycles, parent-child, link resolution
- Materialization: random op sequences → consistent state
- Claim races: concurrent ops, timestamp-based winner selection
- Merge detection: commit-message scan, fallback logic

## Minimal Dependencies

- Do not add external dependencies without justification
- Prefer Go built-ins: `encoding/json`, `flag`, `testing`, `crypto/sha256`
- Hexagonal architecture means boundaries are thin; keep adapters focused

## When Mutation Testing Finds a Survivor

Example: A change to DAG cycle detection logic does not break any test.

1. Run `mutesting` to confirm the mutation survives
2. Write a property test that generates inputs triggering the mutation
3. Fix the property test to fail on the mutant
4. Re-run; mutation should now be caught
```

---

## Implementation Checklist

1. **GVM setup** — install Go 1.22+, set up `$GOPATH`
2. **Project init** — `go mod init github.com/acme/trellis` (or existing module)
3. **Library dependencies** — add libraries via `go get`:
   - `github.com/leanovate/gopter@latest`
   - `github.com/go-mutesting/mutesting@latest`
   - `github.com/spf13/cobra@latest`
   - `github.com/stretchr/testify@latest` (optional)
   - `github.com/google/uuid@latest`
   - Charm ecosystem (if not already present)
4. **Test directories** — organize as `pkg/*_test.go` (core) and `test/` (integration fixtures)
5. **AGENTS.md** — commit to repo root for agent visibility
6. **Example test** — write one property test on DAG validation as proof-of-concept

---

## Success Criteria

- [x] GVM + Go installed and verified
- [x] All libraries in `go.mod`
- [x] AGENTS.md in repo root
- [x] Sample property test for DAG validation passes
- [ ] `mutesting` runs on sample test without errors
- [x] Coverage report generated via `go tool cover`
