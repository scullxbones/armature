# Wave Verification Gate

Do not run `arm merged` `[escape hatch]` until this gate passes. Failed tasks stay `done` so they remain visible.

## Manifest sanity

```bash
echo "Wave: $WAVE_TASK_IDS"
echo "Base SHA: $WAVE_BASE_SHA"
echo "Branch: $WAVE_BRANCH"
echo "Wave type: $WAVE_TYPE"
```

If any variable is unset, stop. Reconstruct from `arm list --status done` and `arm review commits TASK-ID --branch task/TASK-ID`.

## Changed files and auto-promote

```bash
CHANGED_FILES=$(git diff --name-only "$WAVE_BASE_SHA"..HEAD)
if echo "$CHANGED_FILES" | grep -E '\.(go|mod|sum)$' | grep -q . || \
   echo "$CHANGED_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -qvE 'internal/skillsembed'; then
    WAVE_TYPE=code
fi
```

`make check` here is the story-integration publish gate. Workers iterate on `make check-fast`. After the last remediating commit on a task, the worker still runs one task-scoped full gate before confirmation and `done`. This wave run does not replace that.

## Code profile (`WAVE_TYPE=code`)

```bash
go build ./...
make check
arm validate --quiet
arm doctor
```

If `go build` fails and `make` is unavailable:

```bash
go run ./cmd/armature --help
```

## Docs-skill-only profile

```bash
make validate-skills
arm validate --quiet
arm doctor
```

If any `*.go`, `go.mod`, `go.sum`, `Makefile`, `cmd/`, or `internal/` path (outside `internal/skillsembed/`) appears in `$CHANGED_FILES`, promote to the code profile and re-run.

## Bounded remediation (2 attempts)

1. Fix every error and warning, then re-run.
2. If still red, `arm note` the blocker on the story, do not transition, and surface it to the human.

Do not start the next wave or story transition while the gate is red.
