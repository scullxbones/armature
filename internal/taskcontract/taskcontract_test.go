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
