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
3. **Mutation testing is enforcement** — run `gremlins` on core packages; all survivors require test fixes
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

1. Run `gremlins` to confirm: `gremlins unleash ./internal/dag`
2. Write a property test that generates inputs triggering the mutation
3. Fix the property test to fail on the mutant
4. Re-run `gremlins`; mutation should now be caught

## Key Commands

```bash
# Run tests
make test

# Run tests with coverage, generates coverage report in `coverage.html`
make coverage

# Run property tests (runs as part of go test)
go test -run TestProp ./...

# Run mutation testing
make mutate

# Lint
make lint

# Build CLI
make build
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
