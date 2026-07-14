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
        if ! grep -qxF -- "$item" <<< "$census_list"; then
            echo "FAIL: $name '$item' in code but not in census" >&2
            ERRORS=$((ERRORS + 1))
        fi
    done <<< "$code_list"

    # Check items in census but not in code (exact line match, not substring)
    while IFS= read -r item; do
        [[ -z "$item" ]] && continue
        if ! grep -qxF -- "$item" <<< "$code_list"; then
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
# CONFIDENCE STATES CHECK
# ============================================================================
echo "Checking Confidence States..."

# Confidence states aren't enumerated in a const block; they're documented in
# the --confidence flag help text (draft/verified) plus a literal check for
# the "inferred" provenance value in claim.go. Extract from both sources.
CODE_CONFIDENCE=$( {
    grep -h 'confidence level:' "$REPO_ROOT/cmd/armature/create.go" | \
        sed -n 's/.*confidence level: \([a-z]*\) or \([a-z]*\).*/\1\n\2/p'
    grep -oE '"inferred"' "$REPO_ROOT/cmd/armature/claim.go" | tr -d '"'
} | sort -u)

# Extract from census
CENSUS_CONFIDENCE=$(sed -n '/^## Confidence States/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/')

compare_lists "Confidence state" "$CODE_CONFIDENCE" "$CENSUS_CONFIDENCE"

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

# Extract the actual Cobra Use value from each root command constructor. This
# keeps the census aligned with what users can invoke, rather than a guessed
# transformation of Go constructor names.
command_use() {
    local constructor="$1"
    awk -v constructor="$constructor" '
        $0 ~ "^func " constructor "\\(" { in_constructor = 1 }
        in_constructor && /Use:[[:space:]]*"/ {
            if (match($0, /"[^"]+"/)) {
                use = substr($0, RSTART + 1, RLENGTH - 2)
                split(use, parts, " ")
                print parts[1]
                exit
            }
        }
    ' "$REPO_ROOT"/cmd/armature/*.go
}

CODE_CMDS=$(grep -oE 'new[A-Z][A-Za-z]+Cmd\(\)' "$REPO_ROOT/cmd/armature/main.go" | grep -v '^newRootCmd()$' | \
    sed -E 's/^new//; s/Cmd\(\)$//' | while read -r name; do
        command_use "new${name}Cmd"
    done | sort -u)

# Also walk subcommands registered via AddCommand inside each top-level
# constructor (e.g. `review prepare`, `sources add`, `hook run`) so the
# census can be checked against real subcommand names, not just top-level
# command names. Without this, a flag/command mistakenly attributed to a
# nonexistent subcommand (e.g. "review attest") would pass unnoticed.
subcommand_constructors() {
    local constructor="$1"
    awk -v ctor="$constructor" '
        $0 ~ "^func " ctor "\\(" { infunc = 1; next }
        infunc && /^func / { infunc = 0 }
        infunc && /AddCommand\(new[A-Za-z]+Cmd\(\)\)/ {
            if (match($0, /new[A-Za-z]+Cmd/)) {
                print substr($0, RSTART, RLENGTH)
            }
        }
    ' "$REPO_ROOT"/cmd/armature/*.go
}

CODE_SUBCMDS=$(grep -oE 'new[A-Z][A-Za-z]+Cmd\(\)' "$REPO_ROOT/cmd/armature/main.go" | grep -v '^newRootCmd()$' | \
    sed -E 's/^new//; s/Cmd\(\)$//' | while read -r name; do
        parent_use=$(command_use "new${name}Cmd")
        [[ -z "$parent_use" ]] && continue
        subcommand_constructors "new${name}Cmd" | while read -r sub_ctor; do
            sub_use=$(command_use "$sub_ctor")
            [[ -n "$sub_use" ]] && echo "${parent_use} ${sub_use}"
        done
    done | sort -u)

CODE_CMDS=$(printf '%s\n%s\n' "$CODE_CMDS" "$CODE_SUBCMDS" | sed '/^$/d' | sort -u)

# Extract from census (get command names from CLI Commands section)
CENSUS_CMDS=$(sed -n '/^## CLI Commands/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "CLI command" "$CODE_CMDS" "$CENSUS_CMDS"

# ============================================================================
# COMMAND FLAGS CHECK
# ============================================================================
echo "Checking Command Flags..."

# Extract every Cobra flag name from root persistent flags and command-local
# flag definitions. The census deliberately groups shared flags by usage, so
# this validates the documented flag surface as a set.
CODE_FLAGS=$(grep -h 'Flags()\.' "$REPO_ROOT"/cmd/armature/*.go | \
    sed -n 's/.*Flags()\.[A-Za-z]*([^" ]* *"\([^"]*\)".*/--\1/p' | sort -u)

CENSUS_FLAGS=$(sed -n '/^## Command Flags/,/^## Priority Levels/p' "$CENSUS_FILE" | \
    grep '| `--' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "Command flag" "$CODE_FLAGS" "$CENSUS_FLAGS"

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
