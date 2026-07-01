# Quality Gates

## TDD is mandatory

Write failing test → minimum implementation → refactor. No exceptions. No implementation without a test.

## `make check` must pass before every commit/push

```bash
make check   # lint + test + coverage (≥85%) + mutate + validate-skills + build
```

Fix failures; never suppress. Linters: `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`. No `//nolint` without justification. Don't lower coverage (85%) or mutation thresholds (95% mcover, 99% efficacy).

This is the commit/push gate. It's distinct from `arm validate --ci` / `arm doctor`, which is the task-completion sanity check — see [workflow.md](workflow.md).

## Commands

```bash
make build           # build ./bin/arm
make install         # build → ~/.local/bin/arm
make test            # run all tests
make lint            # golangci-lint run ./...
make coverage        # generate coverage.html
make coverage-check  # fail if coverage < 85%
make mutate          # gremlins mutation testing on ./internal
make validate-skills # validate embedded skill source
make skill           # deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
make clean           # remove bin/, dist/, *.out, coverage.html, mutesting-report/, .claude/skills/

go test ./internal/materialize/... -run TestApplyOp   # single test
go test -v ./internal/materialize/...                 # verbose
```

Dependencies: `golangci-lint` and `gremlins` must be on PATH.
