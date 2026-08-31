package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func readEmbedSkill(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(projectRootDir(t), "internal", "skillsembed", "skills", name, "SKILL.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func headingSection(t *testing.T, body, heading, nextHeading string) string {
	t.Helper()
	start := strings.Index(body, heading)
	require.GreaterOrEqual(t, start, 0, "missing heading %q", heading)
	rest := body[start:]
	if nextHeading == "" {
		return rest
	}
	end := strings.Index(rest[len(heading):], nextHeading)
	require.GreaterOrEqual(t, end, 0, "missing following heading %q after %q", nextHeading, heading)
	return rest[:len(heading)+end]
}

func TestReviewerSkillTreatsBundleSetupReportsAsOperational_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	reviewer := readEmbedSkill(t, "armature-reviewer")
	step5b := headingSection(t, reviewer, "### 5b. Self-Validate with `arm review validate`", "### 6. Return the ConformanceAssessment")

	require.Contains(t, step5b, "arm review prepare",
		"5b must classify a valid:false report whose suggestion is to re-run arm review prepare")
	require.Contains(t, step5b, "Validation: error",
		"bundle/setup reports must use the operational no-path shape, not assessment retries")
	require.NotContains(t, step5b, "four attempts",
		"setup reports must not consume the four-run assessment retry cap")
}

func TestReviewerSkillRegeneratesFindingsAfterValidateRewrites_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	reviewer := readEmbedSkill(t, "armature-reviewer")
	step6 := headingSection(t, reviewer, "### 6. Return the ConformanceAssessment", "## Activity Evidence")

	require.Contains(t, step6, "final validated",
		"success chat must be rebuilt from the post-suggestion validated assessment")
	require.Contains(t, strings.ToLower(step6), "actionable findings",
		"validator-driven status changes must refresh actionable findings, not keep the pre-validate list")
}

func TestReviewerSkillMapsBundlePreflightToValidationError_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	reviewer := readEmbedSkill(t, "armature-reviewer")
	step1 := headingSection(t, reviewer, "### 1. Parse and Validate the ReviewBundle", "### 2. Evaluate Definition of Done")

	require.Contains(t, step1, "Validation: error",
		"Step 1 preflight must return a bounded no-path shape the coordinator already recognizes")
	require.Contains(t, step1, "Assessment: not returned")
}

func TestReviewerSkillReadsJSONOperationalFailuresFromStdout_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	reviewer := readEmbedSkill(t, "armature-reviewer")
	step5b := headingSection(t, reviewer, "### 5b. Self-Validate with `arm review validate`", "### 6. Return the ConformanceAssessment")

	require.Contains(t, step5b, ".error.cause",
		"--format json operational failures write {error:...} to stdout; extract .error.cause")
	require.Contains(t, step5b, `"error"`,
		"an object with error and no valid is the operational shape, not a missing JSON object")
}

func TestCoordinatorSkillHandlesNoPathOnConfirmation_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	coord := readEmbedSkill(t, "armature-coordinator")
	confirm := headingSection(t, coord, "6. **Confirmation after remediation", "7. **Inspect confirmation rating")

	require.Contains(t, confirm, "Assessment: not returned",
		"confirmation dispatch must branch on the same no-path shapes as initial collection")
	require.Contains(t, confirm, "Validation: error")
	require.Contains(t, confirm, "Validation: failed")
	require.NotContains(t, confirm, `RESULT_FILE="<path from confirmation reviewer chat>"`,
		"confirmation must not assign a path before checking response shape")
}

func TestCoordinatorSkillStopsOnAnyUnrecoveredNoPath_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	coord := readEmbedSkill(t, "armature-coordinator")
	collect := headingSection(t, coord, "**Check each reviewer's response shape before collecting anything.**", "4. **Record every assessment**")

	require.Contains(t, strings.ToLower(collect), "any unrecovered",
		"a mixed Green + Validation: failed wave must stop before recording")
	require.Contains(t, collect, "RESULT_FILES",
		"nonempty RESULT_FILES is not enough to proceed when a sibling returned no path")
}

func TestCoordinatorSkillRedispatchesAllAfterBundleRefresh_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()
	coord := readEmbedSkill(t, "armature-coordinator")
	collect := headingSection(t, coord, "**Check each reviewer's response shape before collecting anything.**", "4. **Record every assessment**")

	require.Contains(t, collect, "every reviewer whose result will be recorded",
		"after refreshing the bundle, re-dispatch every reviewer that will be recorded")
	require.NotContains(t, collect, "re-dispatch that reviewer **once**",
		"re-dispatching only the failed reviewer leaves other assessments bound to the old bundle")
}
