#!/bin/bash
# Usage: scripts/check-fast.sh [repo-root]
#        BASE=<ref> scripts/check-fast.sh [repo-root]
# Routing table: docs/design/gate-efficiency.md D2.

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
HAS_SURFACE_BUDGET=0

while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    case "$f" in
        *.go)
            HAS_GO=1
            ;;
    esac
    case "$f" in
        skills/*|docs/skills/*|internal/skillsembed/skills/*|.agents/skills/*)
            HAS_SKILLS=1
            ;;
    esac
    case "$f" in
        cmd/*|docs/design/surface-census.md|docs/commands.md)
            HAS_CENSUS_SURFACE=1
            ;;
    esac
    if [[ "$f" == "docs/design/surface-budget-baseline.json" ]] || [[ "$f" =~ ^internal/.+\.go$ ]]; then
        HAS_SURFACE_BUDGET=1
    fi
done <<< "$CHANGED_FILES"

HAS_NON_DOCS=$(( HAS_GO || HAS_SKILLS || HAS_CENSUS_SURFACE || HAS_SURFACE_BUDGET ))

run_step() {
    echo "==> $*"
    "$@"
}

if [[ $HAS_GO -eq 1 ]]; then
    echo "Go changes detected: lint + build + go test on changed packages plus reverse importers"

    CHANGED_GO_FILES=$(echo "$CHANGED_FILES" | grep -E '\.go$' || true)
    CHANGED_PKGS=""
    if [[ -n "$CHANGED_GO_FILES" ]]; then
        CHANGED_PKGS=$(echo "$CHANGED_GO_FILES" | xargs -r -n1 dirname | sort -u | while read -r d; do
            printf './%s\n' "$d"
        done)
    fi

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
    echo "Skills/docs-skills changes detected: validate-skills + validate-doc-examples"
    run_step $MAKE validate-skills
    run_step $MAKE validate-doc-examples
fi

if [[ $HAS_CENSUS_SURFACE -eq 1 ]]; then
    echo "cmd/ or census-surface changes detected: census-drift-check"
    run_step $MAKE census-drift-check
fi

if [[ $HAS_SURFACE_BUDGET -eq 1 ]]; then
    echo "internal/ or surface-budget baseline changes detected: surface-budget-check"
    run_step $MAKE surface-budget-check
fi

if [[ $HAS_NON_DOCS -eq 0 ]]; then
    echo "Docs-only change detected: adr-principles lint only"
    run_step $MAKE adr-principles
fi

echo
echo "check-fast: done"
