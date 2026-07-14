#!/bin/bash
# Real (non-manual) test for census-drift-check.sh.
#
# Verifies:
# 1. The check passes (exit 0) on a clean checkout.
# 2. The check fails (non-zero) and reports the expected message when a
#    surface exists in code but not in the census (injected op type, command,
#    or flag).
#
# This is wired into `make check` via the `test-census-drift-check` target.

set -euo pipefail

REPO_ROOT="${1:-.}"

FAILURES=0

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
FIXTURE="$WORKDIR/fixture"

# Build from the candidate worktree rather than HEAD. Copy every source tree
# the checker reads, rather than overlaying only the checker and census onto a
# historical snapshot.
make_fixture() {
    local destination="$1"
    mkdir -p "$destination"
    tar -C "$REPO_ROOT" -cf - \
        cmd internal docs/design/surface-census.md scripts/census-drift-check.sh | \
        tar -x -C "$destination"
    chmod +x "$destination/scripts/census-drift-check.sh"
}

make_fixture "$FIXTURE"
SCRIPT="$FIXTURE/scripts/census-drift-check.sh"

if ! cmp -s "$REPO_ROOT/cmd/armature/transition.go" "$FIXTURE/cmd/armature/transition.go"; then
    echo "FAIL: fixture does not contain the candidate worktree transition source" >&2
    exit 1
fi

# ----------------------------------------------------------------------------
# Test 1: the candidate checker passes against its clean fixture
# ----------------------------------------------------------------------------
echo "Test 1: census-drift-check passes on a clean fixture..."
CLEAN_OUT=$(mktemp)
if "$SCRIPT" "$FIXTURE" > "$CLEAN_OUT" 2>&1; then
    echo "  PASS"
else
    echo "  FAIL: expected exit 0 on clean tree, got non-zero"
    cat "$CLEAN_OUT"
    FAILURES=$((FAILURES + 1))
fi
rm -f "$CLEAN_OUT"

# ----------------------------------------------------------------------------
# Test 2: an op type in code but not in the census is detected as drift
# ----------------------------------------------------------------------------
echo "Test 2: census-drift-check detects an undocumented op type..."

TYPES_FILE="$FIXTURE/internal/ops/types.go"
if [[ ! -f "$TYPES_FILE" ]]; then
    echo "  FAIL: fixture missing $TYPES_FILE"
    FAILURES=$((FAILURES + 1))
else
    # Inject a fake op type constant that has no corresponding census row.
    awk '
        /^const \(/ && !done { print; print "\tOpFakeDriftSurface = \"fake-drift-surface\""; done=1; next }
        { print }
    ' "$TYPES_FILE" > "$TYPES_FILE.new" && mv "$TYPES_FILE.new" "$TYPES_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented op type is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "Op type 'fake-drift-surface' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

# ----------------------------------------------------------------------------
# Test 3: a changed root Cobra command Use value is detected as drift
# ----------------------------------------------------------------------------
echo "Test 3: census-drift-check detects an undocumented CLI command..."

COMMAND_FILE="$FIXTURE/cmd/armature/dagsum.go"
if [[ ! -f "$COMMAND_FILE" ]]; then
    echo "  FAIL: fixture missing $COMMAND_FILE"
    FAILURES=$((FAILURES + 1))
else
    sed -i 's/Use:   "dag-summary"/Use:   "fake-drift-command"/' "$COMMAND_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented command is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "CLI command 'fake-drift-command' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected command drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

# ----------------------------------------------------------------------------
# Test 4: a Cobra flag missing from the census is detected as drift
# ----------------------------------------------------------------------------
echo "Test 4: census-drift-check detects an undocumented command flag..."

FLAG_FILE="$FIXTURE/cmd/armature/link.go"
if [[ ! -f "$FLAG_FILE" ]]; then
    echo "  FAIL: fixture missing $FLAG_FILE"
    FAILURES=$((FAILURES + 1))
else
    sed -i 's/"source", "", "source issue ID"/"fake-drift-flag", "", "source issue ID"/' "$FLAG_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented flag is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "Command flag '--fake-drift-flag' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected flag drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

# ----------------------------------------------------------------------------
# Test 5: a phantom command name in the census (that doesn't exist in code,
# including as a subcommand) is detected as drift
# ----------------------------------------------------------------------------
echo "Test 5: census-drift-check detects a phantom census command..."

CENSUS_FIXTURE="$FIXTURE/docs/design/surface-census.md"
if [[ ! -f "$CENSUS_FIXTURE" ]]; then
    echo "  FAIL: fixture missing $CENSUS_FIXTURE"
    FAILURES=$((FAILURES + 1))
else
    # Inject a phantom subcommand row that has no matching AddCommand/Use in
    # code, mirroring the real "review attest" drift this regression guards
    # against (a phantom subcommand name that was never a real command).
    sed -i 's/^| `review record` |/| `review attest` |/' "$CENSUS_FIXTURE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when a phantom census command is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "CLI command 'review attest' in census but not in code" <<< "$OUTPUT"; then
        echo "  FAIL: expected phantom command drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi

# Restore the candidate census for any subsequent tests.
    cp "$REPO_ROOT/docs/design/surface-census.md" "$CENSUS_FIXTURE"
fi

# ----------------------------------------------------------------------------
# Test 6: all forms of missing or incorrect transition --force ownership are
# detected. The fixture's candidate source was verified above, rather than
# relying on a historical HEAD archive plus a partial overlay.
# ----------------------------------------------------------------------------
echo "Test 6: census-drift-check detects transition --force ownership drift..."

OWNERSHIP_FIXTURE="$WORKDIR/ownership-fixture"
make_fixture "$OWNERSHIP_FIXTURE"

assert_transition_force_drift() {
    local name="$1"
    local expected="transition|--force"
    cp "$REPO_ROOT/docs/design/surface-census.md" "$OWNERSHIP_FIXTURE/docs/design/surface-census.md"
    case "$name" in
        omitted)
            sed -i 's/, transition | bool/, | bool/' "$OWNERSHIP_FIXTURE/docs/design/surface-census.md"
            ;;
        removed)
            sed -i '/^| `--force` |/d' "$OWNERSHIP_FIXTURE/docs/design/surface-census.md"
            expected="Command flag '--force' in code but not in census"
            ;;
        misassigned)
            sed -i 's/, transition | bool/, scope-delete | bool/' "$OWNERSHIP_FIXTURE/docs/design/surface-census.md"
            ;;
    esac

    set +e
    OUTPUT=$("$OWNERSHIP_FIXTURE/scripts/census-drift-check.sh" "$OWNERSHIP_FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when transition --force ownership is $name"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "$expected" <<< "$OUTPUT"; then
        echo "  FAIL: expected transition --force ownership drift message not found for $name"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS: $name"
    fi
}

assert_transition_force_drift "omitted"
assert_transition_force_drift "removed"
assert_transition_force_drift "misassigned"

# ----------------------------------------------------------------------------
# Test 7: a materialized Issue field missing from the census is detected
# ----------------------------------------------------------------------------
echo "Test 7: census-drift-check detects an undocumented Issue field..."

FIELDS_FIXTURE="$WORKDIR/fields-fixture"
make_fixture "$FIELDS_FIXTURE"

sed -i '/^type Issue struct {/a\\	FakeDriftField string `json:"fake_drift_field,omitempty"`' \
    "$FIELDS_FIXTURE/internal/materialize/state.go"

set +e
OUTPUT=$("$FIELDS_FIXTURE/scripts/census-drift-check.sh" "$FIELDS_FIXTURE" 2>&1)
STATUS=$?
set -e

if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit when an undocumented Issue field is injected"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "Issue field 'fake_drift_field' in code but not in census" <<< "$OUTPUT"; then
    echo "  FAIL: expected Issue field drift message not found in output"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 8: a flag READ (cmd.Flags().GetString) inside a command's RunE must not
# be credited as flag ownership. The ownership awk pattern historically matched
# any `Flags().<Word>(` call, including reads like GetString/Changed/Lookup/Set,
# not just definition-style calls (String/StringVar/Bool/...). A read of a flag
# that isn't even defined (a "phantom" flag) must not cause the checker to
# treat that command as the flag's owner / require a census row for it.
# ----------------------------------------------------------------------------
echo "Test 8: census-drift-check ignores flag reads (GetString/Changed/etc) when determining ownership..."

PHANTOM_FIXTURE="$WORKDIR/phantom-fixture"
make_fixture "$PHANTOM_FIXTURE"

sed -i 's/repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")/repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")\n\t\t\tphantomVal, _ := cmd.Flags().GetString("phantom-flag")\n\t\t\t_ = phantomVal/' \
    "$PHANTOM_FIXTURE/cmd/armature/bootstrap.go"

set +e
OUTPUT=$("$PHANTOM_FIXTURE/scripts/census-drift-check.sh" "$PHANTOM_FIXTURE" 2>&1)
STATUS=$?
set -e

if [[ $STATUS -ne 0 ]]; then
    echo "  FAIL: expected exit 0 — a flag read must not be credited as flag ownership"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif grep -q "phantom-flag" <<< "$OUTPUT"; then
    echo "  FAIL: phantom-flag read was incorrectly credited as ownership"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All census-drift-check tests passed"
    exit 0
else
    echo "FAIL: $FAILURES census-drift-check test(s) failed"
    exit 1
fi
