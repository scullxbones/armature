package taskcontract_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/taskcontract"
)

func TestCheckTaskContract_DoctorRunWiring_REQ_TOPTIER_S18_T0(t *testing.T) {
	t.Parallel()

	t.Run("gains_check_without_doctor_go_violates", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "LNGHZN-S7-T2",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D9 so the config file can never silently lie again",
			Scope:            []string{"internal/config/strict.go", "internal/doctor/config_check.go"},
		})
		require.Len(t, got, 1)
		assert.Equal(t, taskcontract.RuleDoctorRunWiring, got[0].Rule)
		assert.Equal(t, "LNGHZN-S7-T2", got[0].TaskID)
		assert.Contains(t, got[0].Message, taskcontract.DoctorRunWiringPath)
	})

	t.Run("gains_check_with_doctor_go_ok", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "LNGHZN-S7-T6",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D10",
			Scope:            []string{"internal/doctor/doctor.go", "internal/doctor/doctor_test.go"},
		})
		assert.Empty(t, got)
	})

	t.Run("doctor_glob_covers_wiring_file", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "S12-T2",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "gains check D8",
			Scope:            []string{"internal/doctor/**"},
		})
		assert.Empty(t, got)
	})

	t.Run("arm_doctor_completion_ritual_is_not_a_wiring_claim", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "SOME-T1",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "make check green; arm doctor and arm validate --ci before done",
			Scope:            []string{"internal/foo/foo.go"},
		})
		assert.Empty(t, got, "mentioning arm doctor as a completion check must not require doctor.go")
	})

	t.Run("arm_doctor_before_done_ritual_is_not_a_wiring_claim", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "SOME-T2",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "run arm doctor before done",
			Scope:            []string{"internal/foo/foo.go"},
		})
		assert.Empty(t, got, "arm doctor as a pre-done quality-gate command is ritual, not a behavior claim")
	})

	t.Run("arm_doctor_behavior_claim_without_check_id_requires_wiring", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "PROD-T1",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor reports malformed config as an error",
			Scope:            []string{"internal/config/strict.go"},
		})
		require.Len(t, got, 1)
		assert.Equal(t, taskcontract.RuleDoctorRunWiring, got[0].Rule)
		assert.Equal(t, "PROD-T1", got[0].TaskID)
		assert.Contains(t, got[0].Message, taskcontract.DoctorRunWiringPath)
	})

	t.Run("behavior_claim_plus_ritual_still_requires_wiring", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "PROD-T2",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor reports malformed config as an error; arm doctor and arm validate --ci before done",
			Scope:            []string{"internal/config/strict.go"},
		})
		require.Len(t, got, 1)
		assert.Equal(t, taskcontract.RuleDoctorRunWiring, got[0].Rule)
	})

	t.Run("helper_only_dod_does_not_require_run_wiring", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "HELPER-T1",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D9 as an exported helper, not wired into Run",
			Scope:            []string{"internal/doctor/config_check.go"},
		})
		assert.Empty(t, got)
	})

	t.Run("bare_exported_helper_does_not_opt_out_of_run_wiring", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "HELPER-T2",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D11 through an exported helper",
			Scope:            []string{"internal/doctor/config_check.go"},
		})
		require.Len(t, got, 1)
		assert.Equal(t, taskcontract.RuleDoctorRunWiring, got[0].Rule)
		assert.Equal(t, "HELPER-T2", got[0].TaskID)
	})

	t.Run("helper_only_phrase_opts_out", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "HELPER-T3",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D11 helper-only",
			Scope:            []string{"internal/doctor/config_check.go"},
		})
		assert.Empty(t, got)
	})

	t.Run("non_task_skipped", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "STORY-1",
			Type:             "story",
			Status:           "open",
			DefinitionOfDone: "arm doctor gains check D9",
			Scope:            []string{"internal/doctor/**"},
		})
		assert.Empty(t, got)
	})

	t.Run("readme_pointer_to_doctor_is_not_a_wiring_claim", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:     "TOPTIER-S15-T2",
			Type:   "task",
			Status: "open",
			DefinitionOfDone: "README quickstart gains an If something goes wrong appendix covering " +
				"gopls/LSP false positives, checked-out-branch Managed Worktree failures, and worktree " +
				"leak / wrong-checkout classes from docs/dogfood/findings/themes/git-worktree-friction/README.md, " +
				"plus a pointer to arm doctor --explain and D9 Unrecognized Managed Worktree for doctor-visible cases.",
			Scope: []string{"README.md"},
		})
		assert.Empty(t, got, "docs that point at arm doctor --explain must not require doctor.go")
	})

	t.Run("validate_e14_meta_dod_is_not_a_wiring_claim", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:     "TOPTIER-S18-T2",
			Type:   "task",
			Status: "open",
			DefinitionOfDone: "arm validate Graph Finding E14 when task DoD claims arm doctor/gains check Dn " +
				"whose wiring file is absent from Scope; unit-only Acceptance cannot stand alone beside CLI DoD. " +
				"Tests cover S7-T2 fixture. No S14 dependency.",
			Scope: []string{"internal/validate/validate.go", "internal/validate/validate_test.go"},
		})
		assert.Empty(t, got, "the E14 validate rule itself must not require doctor.go")
	})

	t.Run("wire_dn_into_run_without_doctor_go_violates", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "WIRE-T1",
			Type:             "task",
			Status:           "open",
			DefinitionOfDone: "wire D10 into Run",
			Scope:            []string{"internal/doctor/config_check.go"},
		})
		require.Len(t, got, 1)
		assert.Equal(t, taskcontract.RuleDoctorRunWiring, got[0].Rule)
	})

	t.Run("terminal_task_skipped", func(t *testing.T) {
		t.Parallel()
		got := taskcontract.CheckTaskContract(taskcontract.Task{
			ID:               "OLD-T1",
			Type:             "task",
			Status:           "merged",
			DefinitionOfDone: "arm doctor gains check D9",
			Scope:            []string{"internal/foo.go"},
		})
		assert.Empty(t, got)
	})
}
