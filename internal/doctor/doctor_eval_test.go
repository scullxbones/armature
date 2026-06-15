package doctor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateD1GitDivergenceConsumesCollectedSignals(t *testing.T) {
	t.Parallel()
	finding := EvaluateD1GitDivergence([]string{
		"feat(TASK-001): commit",
	}, map[string]string{
		"TASK-001": "open",
	})

	assert.Equal(t, SeverityWarning, finding.Severity)
	assert.Contains(t, finding.Items[0], "TASK-001")
}
