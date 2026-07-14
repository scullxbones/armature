#!/bin/bash
# Test fixture for census-drift-check.sh
#
# This script demonstrates how the census-drift-check detects surface drift.
#
# The check verifies that:
# 1. All CLI commands defined in cmd/armature/main.go exist in the census
# 2. All issue types in internal/issuetype/ exist in the census
# 3. All statuses in internal/ops/types.go exist in the census
# 4. All op types in internal/ops/types.go exist in the census
#
# Example drift scenarios:
#
# Scenario 1: Adding a new command without census entry
#   - Add newExampleCmd() to cmd/armature/main.go
#   - Add root.AddCommand(exampleCmd) to main()
#   - Run: scripts/census-drift-check.sh
#   - Expected: FAIL with "CLI command 'example' in code but not in census"
#
# Scenario 2: Adding an issue type without census entry
#   - Add "example": true to validTypes map in internal/issuetype/issuetype.go
#   - Run: scripts/census-drift-check.sh
#   - Expected: FAIL with "Issue type 'example' in code but not in census"
#
# Scenario 3: Adding an op type without census entry
#   - Add OpExample = "example" to internal/ops/types.go const block
#   - Run: scripts/census-drift-check.sh
#   - Expected: FAIL with "Op type 'example' in code but not in census"

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/census-drift-check.sh"

echo "Census Drift Check Test"
echo "======================"
echo ""
echo "Running census-drift-check on current codebase..."
echo ""

# Run the check - should pass on clean codebase
if "$SCRIPT" "$REPO_ROOT"; then
    echo ""
    echo "✓ Test passed: No census drift detected on current codebase"
    echo ""
    echo "To test drift detection:"
    echo "1. Add a new command (e.g., newTestCmd) to cmd/armature/main.go"
    echo "2. Run: $SCRIPT"
    echo "3. Expected: FAIL with 'CLI command in code but not in census'"
    exit 0
else
    echo ""
    echo "✗ Test failed: Census drift detected (see above for details)"
    exit 1
fi
