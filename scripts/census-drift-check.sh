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
# ISSUE FIELDS CHECK
# ============================================================================
echo "Checking Issue Fields..."

# The census explicitly inventories the fields materialized on Issue. Compare
# its JSON field names to the source of truth rather than only checking the
# documented row count.
CODE_ISSUE_FIELDS=$(awk '
    /^type Issue struct \{/ { in_issue = 1; next }
    in_issue && /^}/ { exit }
    in_issue {
        if (match($0, /json:"[^"]+"/)) {
            value = substr($0, RSTART + 6, RLENGTH - 7)
            sub(/,.*/, "", value)
            print value
        }
    }
' "$REPO_ROOT/internal/materialize/state.go" | sort -u)

CENSUS_ISSUE_FIELDS=$(sed -n '/^## Issue Fields/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '^| `' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "Issue field" "$CODE_ISSUE_FIELDS" "$CENSUS_ISSUE_FIELDS"

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

# Check root persistent flags independently: they are inherited by every
# command, so their ownership is the root command rather than a local command.
CODE_ROOT_FLAGS=$(sed -n 's/.*PersistentFlags()\.[A-Za-z]*([^" ]* *"\([^"]*\)".*/--\1/p' \
    "$REPO_ROOT/cmd/armature/main.go" | sort -u)
CENSUS_ROOT_FLAGS=$(sed -n '/^### Universal\/Root Flags/,/^### /p' "$CENSUS_FILE" | \
    grep '^| `--' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)
compare_lists "Root persistent flag" "$CODE_ROOT_FLAGS" "$CENSUS_ROOT_FLAGS"

# For command-local flags, compare command/flag pairs, not merely the global
# flag-name set. This catches a real but wrongly attributed flag row.
CONSTRUCTOR_PATHS=$(grep -oE 'new[A-Z][A-Za-z]+Cmd\(\)' "$REPO_ROOT/cmd/armature/main.go" | \
    grep -v '^newRootCmd()$' | sed -E 's/\(\)$//' | while read -r ctor; do
        use=$(command_use "$ctor")
        [[ -n "$use" ]] && printf '%s|%s\n' "$ctor" "$use"
    done)

CONSTRUCTOR_PATHS_TMP=$(mktemp)
trap 'rm -f "$CONSTRUCTOR_PATHS_TMP"' EXIT
while IFS='|' read -r parent_ctor parent_use; do
    [[ -z "$parent_ctor" ]] && continue
    while read -r sub_ctor; do
        sub_use=$(command_use "$sub_ctor")
        [[ -n "$sub_use" ]] && printf '%s|%s %s\n' "$sub_ctor" "$parent_use" "$sub_use"
    done < <(subcommand_constructors "$parent_ctor")
done <<< "$CONSTRUCTOR_PATHS" >> "$CONSTRUCTOR_PATHS_TMP"
CONSTRUCTOR_PATHS=$(cat "$CONSTRUCTOR_PATHS_TMP"; printf '%s\n' "$CONSTRUCTOR_PATHS")
rm -f "$CONSTRUCTOR_PATHS_TMP"
trap - EXIT

CODE_FLAG_OWNERS=$(while IFS='|' read -r constructor command; do
    [[ -z "$constructor" ]] && continue
    awk -v ctor="$constructor" -v command="$command" '
        $0 ~ "^func " ctor "\\(" { in_constructor = 1; next }
        in_constructor && /^func / { exit }
        in_constructor && /Flags\(\)\.[A-Za-z]*\(/ {
            # Only credit ownership from definition-style calls (String, StringVar,
            # StringVarP, Bool, BoolVar, Int, StringSlice, etc — including the P
            # suffix variant). Explicitly exclude read-style calls: Get*, Changed,
            # Set, Lookup. A command merely reading a flag value (e.g. inside a
            # RunE closure via cmd.Flags().GetString(...)) does not own that flag.
            if (match($0, /Flags\(\)\.([A-Za-z]+)\(/)) {
                call = substr($0, RSTART, RLENGTH)
                sub(/^Flags\(\)\./, "", call)
                sub(/\($/, "", call)
                if (call !~ /^Get/ && call != "Changed" && call != "Set" && call != "Lookup") {
                    if (match($0, /"[^"]+"/)) {
                        flag = substr($0, RSTART + 1, RLENGTH - 2)
                        print command "|--" flag
                    }
                }
            }
        }
    ' "$REPO_ROOT"/cmd/armature/*.go
done <<< "$CONSTRUCTOR_PATHS" | sort -u)

# A command cell can contain a comma-separated list. Ignore intentionally
# non-enumerable prose ("etc.") but validate every named canonical command.
CENSUS_FLAG_OWNERS=$(sed -n '/^## Command Flags/,/^## Priority Levels/p' "$CENSUS_FILE" | \
    awk -F'|' '
        /^### Universal\/Root Flags/ { command_table = 0 }
        /^\| Flag \| Command\(s\)/ { command_table = 1; next }
        /^\| `--/ {
            if (!command_table) next
            flag = $2; commands = $3
            gsub(/^[[:space:]]*`|`[[:space:]]*$/, "", flag)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", commands)
            count = split(commands, parts, ",")
            for (i = 1; i <= count; i++) {
                command = parts[i]
                gsub(/^[[:space:]]+|[[:space:]]+$/, "", command)
                if (command != "" && command !~ /etc\./) print command "|" flag
            }
        }
    ' | sort -u)

CODE_FLAGS=$(cut -d'|' -f2 <<< "$CODE_FLAG_OWNERS" | sort -u)
CENSUS_FLAGS=$(cut -d'|' -f2 <<< "$CENSUS_FLAG_OWNERS" | sort -u)
compare_lists "Command flag" "$CODE_FLAGS" "$CENSUS_FLAGS"

# Some census rows intentionally use "etc." rather than enumerating every
# owner. Restrict pair-level comparison to rows with complete ownership lists;
# the name-level comparison above still covers every command flag.
CENSUS_ENUMERATED_FLAGS=$(sed -n '/^## Command Flags/,/^## Priority Levels/p' "$CENSUS_FILE" | \
    awk -F'|' '/^\| `--/ && $0 !~ /etc\./ { flag = $2; gsub(/^[[:space:]]*`|`[[:space:]]*$/, "", flag); print flag }' | sort -u)
CODE_ENUMERATED_FLAG_OWNERS=$(while IFS= read -r pair; do
    [[ -z "$pair" ]] && continue
    flag=${pair##*|}
    if grep -qxF -- "$flag" <<< "$CENSUS_ENUMERATED_FLAGS"; then
        printf '%s\n' "$pair"
    fi
done <<< "$CODE_FLAG_OWNERS")
CENSUS_ENUMERATED_FLAG_OWNERS=$(while IFS= read -r pair; do
    [[ -z "$pair" ]] && continue
    flag=${pair##*|}
    if grep -qxF -- "$flag" <<< "$CENSUS_ENUMERATED_FLAGS"; then
        printf '%s\n' "$pair"
    fi
done <<< "$CENSUS_FLAG_OWNERS")

# Comparing the full command/flag pair in both directions catches not only a
# flag attributed to the wrong command, but also a real command owner omitted
# from a shared census row (for example, `transition|--force`).
compare_lists "Command flag ownership" "$CODE_ENUMERATED_FLAG_OWNERS" "$CENSUS_ENUMERATED_FLAG_OWNERS"

# ============================================================================
# RELATIONSHIP TYPES CHECK
# ============================================================================
echo "Checking Relationship Types..."

# Extract accepted rel values from applyLink's op.Payload.Rel == "..." branches.
CODE_RELS=$(awk '
    /^func \(s \*State\) applyLink\(/ { in_func = 1; next }
    in_func && /^}/ { exit }
    in_func && /op\.Payload\.Rel (==|!=) "/ {
        if (match($0, /op\.Payload\.Rel (==|!=) "[^"]+"/)) {
            s = substr($0, RSTART, RLENGTH)
            sub(/^op\.Payload\.Rel (==|!=) "/, "", s)
            sub(/"$/, "", s)
            print s
        }
    }
' "$REPO_ROOT/internal/materialize/engine.go" | sort -u)

CENSUS_RELS=$(sed -n '/^## Relationship Types/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

# The census also documents `blocks` as a derived/output-only relationship type
# (never a valid --rel input). Only compare accepted *inputs* here, since
# CODE_RELS is sourced from applyLink's input validation branches. A row counts
# as an accepted input only if its Notes column doesn't mark it derived/output-only.
CENSUS_INPUT_RELS=$(sed -n '/^## Relationship Types/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | grep -vi 'derived/output-only' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "Relationship type (accepted input)" "$CODE_RELS" "$CENSUS_INPUT_RELS"

# ============================================================================
# PROVIDER TYPES CHECK
# ============================================================================
echo "Checking Provider Types..."

CODE_PROVIDERS=$(awk '
    /func \(r \*DefaultProviderRegistry\) ProviderForType\(/ { in_func = 1; next }
    in_func && /^}/ { exit }
    in_func && /case "/ {
        if (match($0, /case "[^"]+"/)) {
            s = substr($0, RSTART, RLENGTH)
            sub(/^case "/, "", s)
            sub(/"$/, "", s)
            print s
        }
    }
' "$REPO_ROOT/internal/sources/lifecycle.go" | sort -u)

CENSUS_PROVIDERS=$(sed -n '/^## Provider Types/,/^## [^#]/p' "$CENSUS_FILE" | \
    grep '| `' | sed 's/^| `\([^`]*\)`.*/\1/' | sort -u)

compare_lists "Provider type" "$CODE_PROVIDERS" "$CENSUS_PROVIDERS"

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
