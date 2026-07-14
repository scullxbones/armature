#!/bin/bash
# Census drift check: verify that code surfaces match docs/design/surface-census.md
# Fails non-zero if surfaces exist in code without matching census rows.

set -euo pipefail

CENSUS_FILE="${1:-.}/docs/design/surface-census.md"
REPO_ROOT="${1:-.}"

if [[ ! -f "$CENSUS_FILE" ]]; then
    echo "FAIL: census file not found: $CENSUS_FILE" >&2
    exit 1
fi

ERRORS=0

# Helper to compare two lists and report differences
compare_lists() {
    local name="$1"
    local code_list="$2"
    local census_list="$3"

    # Check items in code but not in census (exact line match, not substring)
    while IFS= read -r item; do
        [[ -z "$item" ]] && continue
        if ! grep -qxF "$item" <<< "$census_list"; then
            echo "FAIL: $name '$item' in code but not in census" >&2
            ERRORS=$((ERRORS + 1))
        fi
    done <<< "$code_list"

    # Check items in census but not in code (exact line match, not substring)
    while IFS= read -r item; do
        [[ -z "$item" ]] && continue
        if ! grep -qxF "$item" <<< "$code_list"; then
            echo "FAIL: $name '$item' in census but not in code" >&2
            ERRORS=$((ERRORS + 1))
        fi
    done <<< "$census_list"
}

# ============================================================================
# ISSUE TYPES CHECK
# ============================================================================
echo "Checking Issue Types..."

# Extract from code
CODE_TYPES=$(sed -n '/^var validTypes = map/,/^}/p' "$REPO_ROOT/internal/issuetype/issuetype.go" | \
    grep -E '[:=].*true' | sed 's/.*"\([^"]*\)".*/\1/')

# Extract from census: first column of Issue Types table
CENSUS_TYPES=$(sed -n '/^## Issue Types/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/')

compare_lists "Issue type" "$CODE_TYPES" "$CENSUS_TYPES"

# ============================================================================
# ISSUE STATUSES CHECK
# ============================================================================
echo "Checking Issue Statuses..."

# Extract from code
CODE_STATUSES=$(grep -E '^\s+Status\w+\s*=' "$REPO_ROOT/internal/ops/types.go" | \
    sed 's/.*= "\([^"]*\)".*/\1/')

# Extract from census
CENSUS_STATUSES=$(sed -n '/^## Issue Statuses/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/')

compare_lists "Status" "$CODE_STATUSES" "$CENSUS_STATUSES"

# ============================================================================
# OP TYPES CHECK
# ============================================================================
echo "Checking Op Types..."

# Extract from code
CODE_OPS=$(grep -E '^\s+Op\w+\s*=' "$REPO_ROOT/internal/ops/types.go" | \
    sed 's/.*= "\([^"]*\)".*/\1/')

# Extract from census
CENSUS_OPS=$(sed -n '/^## Operation Types/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/')

compare_lists "Op type" "$CODE_OPS" "$CENSUS_OPS"

# ============================================================================
# CLI COMMANDS CHECK
# ============================================================================
echo "Checking CLI Commands..."

# Build a mapping of function names to command names based on the census
declare -A cmd_map=(
    [Ready]=ready
    [Claim]=claim
    [Transition]=transition
    [Unassign]=unassign
    [Reopen]=reopen
    [Heartbeat]=heartbeat
    [Note]=note
    [Decision]=decision
    [Amend]=amend
    [Confirm]=confirm
    [Assign]=assign
    [DAGSummary]=dagsum
    [DAGTransition]="dag-transition"
    [DecomposeApply]="decompose apply"
    [DecomposeRevert]="decompose revert"
    [DecomposeContext]="decompose context"
    [Link]=link
    [Unlink]=unlink
    [Sync]=sync
    [PushOps]="push-ops"
    [Merged]=merged
    [Materialize]=materialize
    [Import]=import
    [StaleReview]="stale-review"
    [Version]=version
    [WorkerInit]="worker-init"
    [Bootstrap]=bootstrap
    [Create]=create
    [Reparent]=reparent
    [Validate]=validate
    [RenderContext]="render-context"
    [Log]=log
    [Workers]=workers
    [Sources]=sources
    [SourceLink]="source-link"
    [AcceptCitation]="accept-citation"
    [Show]=show
    [List]=list
    [ScopeRename]="scope-rename"
    [ScopeDelete]="scope-delete"
    [Doctor]=doctor
    [Completion]=completion
    [Hook]=hook
    [TUI]=tui
    [ContextHistory]="context-history"
    [HarnessHook]="harness-hook"
    [Review]=review
)

# Extract commands from code
CODE_CMDS=$(grep -oE 'new[A-Z][A-Za-z]+Cmd\(\)' "$REPO_ROOT/cmd/armature/main.go" | \
    sed 's/^new//' | sed 's/Cmd()$//' | while read name; do
        if [[ -v cmd_map[$name] ]]; then
            echo "${cmd_map[$name]}"
        else
            echo "$name" | sed 's/\([A-Z]\)/-\L\1/g' | sed 's/^-//'
        fi
    done | grep -v '^root$' | sort -u)

# Extract from census (get command names from CLI Commands section)
CENSUS_CMDS=$(sed -n '/^## CLI Commands/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "CLI command" "$CODE_CMDS" "$CENSUS_CMDS"

# ============================================================================
# SUMMARY
# ============================================================================
if [[ $ERRORS -eq 0 ]]; then
    echo "✓ Census drift check passed"
    exit 0
else
    echo "FAIL: Found $ERRORS census drift error(s)" >&2
    exit 1
fi
