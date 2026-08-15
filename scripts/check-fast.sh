#!/bin/bash
# check-fast: deterministic, diff-routed gate for iteration/remediation.
#
# Computes changed files against BASE (default: merge-base HEAD origin/main,
# overridable via BASE=<ref>) and runs only the steps implied by the changed
# surfaces, per docs/design/gate-efficiency.md D2:
#
#   **/*.go                                          -> lint + build + go test
#                                                        on changed packages
#                                                        plus reverse importers
#   skills/**, docs/skills/**                         -> validate-skills,
#                                                        validate-doc-examples
#   cmd/**, docs/design/surface-census.md,
#   docs/commands.md                                  -> census-drift-check
#   docs only (none of the above)                     -> adr-principles lint
#
# This is the fast gate: it MUST NOT run mutation testing, full coverage, or
# crosscompile. Those remain exclusive to `make check` (the publish gate).
#
# Usage: scripts/check-fast.sh [repo-root]
#        BASE=<ref> scripts/check-fast.sh [repo-root]

set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

GO="${GO:-go}"
MAKE="${MAKE:-make}"

BASE="${BASE:-}"
if [[ -z "$BASE" ]]; then
    if ! BASE=$(git merge-base HEAD origin/main 2>/dev/null); then
        echo "FAIL: could not compute merge-base HEAD origin/main; set BASE= explicitly" >&2
        exit 1
    fi
fi

CHANGED_FILES=$(git diff --name-only "$BASE" -- 2>/dev/null || true)
# Include untracked files so uncommitted new files participate in routing.
CHANGED_FILES=$(printf '%s\n%s\n' "$CHANGED_FILES" "$(git ls-files --others --exclude-standard)" | sed '/^$/d' | sort -u)

if [[ -z "$CHANGED_FILES" ]]; then
    echo "No changes detected against $BASE; nothing to route."
    exit 0
fi

echo "check-fast: routing against BASE=$BASE"
echo "Changed files:"
echo "$CHANGED_FILES" | sed 's/^/  /'
echo

HAS_GO=0
HAS_SKILLS=0
HAS_CENSUS_SURFACE=0
HAS_NON_DOCS=0

while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    case "$f" in
        *.go)
            HAS_GO=1
            HAS_NON_DOCS=1
            ;;
    esac
    case "$f" in
        skills/*|docs/skills/*)
            HAS_SKILLS=1
            HAS_NON_DOCS=1
            ;;
    esac
    case "$f" in
        cmd/*|docs/design/surface-census.md|docs/commands.md)
            HAS_CENSUS_SURFACE=1
            HAS_NON_DOCS=1
            ;;
    esac
    case "$f" in
        docs/*) ;;
        *) HAS_NON_DOCS=1 ;;
    esac
done <<< "$CHANGED_FILES"

RAN_ANY=0

run_step() {
    echo "==> $*"
    "$@"
}

if [[ $HAS_GO -eq 1 ]]; then
    RAN_ANY=1
    echo "Go changes detected: lint + build + go test on changed packages plus reverse importers"

    CHANGED_GO_FILES=$(echo "$CHANGED_FILES" | grep -E '\.go$' || true)
    CHANGED_PKGS=""
    if [[ -n "$CHANGED_GO_FILES" ]]; then
        CHANGED_PKGS=$(echo "$CHANGED_GO_FILES" | xargs -r -n1 dirname | sort -u | while read -r d; do
            printf './%s\n' "$d"
        done)
    fi

    # Reverse importers: any package in the module whose dependency set
    # includes a changed package's import path.
    REVERSE_PKGS=""
    if [[ -n "$CHANGED_PKGS" ]]; then
        CHANGED_IMPORT_PATHS=$(while IFS= read -r pkg; do
            [[ -z "$pkg" ]] && continue
            $GO list "$pkg" 2>/dev/null || true
        done <<< "$CHANGED_PKGS" | sed '/^$/d' | sort -u)

        REVERSE_PKGS=$($GO list -deps -f '{{.ImportPath}} {{join .Deps " "}}' ./... 2>/dev/null | \
            while read -r importpath deps; do
                for cip in $CHANGED_IMPORT_PATHS; do
                    if [[ " $deps " == *" $cip "* ]]; then
                        echo "$importpath"
                        break
                    fi
                done
            done | sort -u)
    fi

    TEST_PKGS=$(printf '%s\n%s\n' "$CHANGED_PKGS" "$REVERSE_PKGS" | sed '/^$/d' | sort -u)
    # Normalize changed package dirs to import paths for go test/lint.
    TEST_IMPORT_PATHS=$(while IFS= read -r pkg; do
        [[ -z "$pkg" ]] && continue
        if [[ "$pkg" == ./* ]]; then
            $GO list "$pkg" 2>/dev/null || true
        else
            echo "$pkg"
        fi
    done <<< "$TEST_PKGS" | sed '/^$/d' | sort -u)

    if [[ -n "$TEST_IMPORT_PATHS" ]]; then
        run_step $MAKE build

        # golangci-lint wants filesystem-relative dir patterns (./pkg1), not
        # bare import paths, or it mis-resolves them relative to cwd. Convert.
        TEST_DIR_PATTERNS=$(while IFS= read -r importpath; do
            [[ -z "$importpath" ]] && continue
            dir=$($GO list -f '{{.Dir}}' "$importpath" 2>/dev/null || true)
            [[ -z "$dir" ]] && continue
            rel=$(realpath --relative-to="$(pwd)" "$dir" 2>/dev/null || echo "$dir")
            echo "./$rel"
        done <<< "$TEST_IMPORT_PATHS" | sed '/^$/d' | sort -u)

        if command -v golangci-lint >/dev/null 2>&1; then
            echo "==> golangci-lint run $TEST_DIR_PATTERNS"
            XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run $TEST_DIR_PATTERNS
        else
            echo "golangci-lint not found; skipping lint step (install to enforce locally)" >&2
        fi
        echo "==> go test $TEST_IMPORT_PATHS"
        ARM_BIN="$(pwd)/bin/arm" $GO test -count=1 $TEST_IMPORT_PATHS
    fi
fi

if [[ $HAS_SKILLS -eq 1 ]]; then
    RAN_ANY=1
    echo "Skills/docs-skills changes detected: validate-skills + validate-doc-examples"
    run_step $MAKE validate-skills
    run_step $MAKE validate-doc-examples
fi

if [[ $HAS_CENSUS_SURFACE -eq 1 ]]; then
    RAN_ANY=1
    echo "cmd/ or census-surface changes detected: census-drift-check"
    run_step $MAKE census-drift-check
fi

if [[ $HAS_NON_DOCS -eq 0 ]]; then
    RAN_ANY=1
    echo "Docs-only change detected: adr-principles lint only"
    run_step $MAKE adr-principles
fi

if [[ $RAN_ANY -eq 0 ]]; then
    echo "No routable surface matched; nothing to run."
fi

echo
echo "check-fast: done"
