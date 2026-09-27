package doctor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckD1GitDivergence_GitLogUnavailable_FailOpen_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	finding := checkD1GitDivergence("/nonexistent/path", nil)
	assert.Equal(t, "D1", finding.Check)
	assert.Equal(t, SeverityOK, finding.Severity)
	assert.Equal(t, "Git log unavailable; D1 not checked", finding.Message)
}

func TestEvaluateD1GitDivergenceConsumesCollectedSignals(t *testing.T) {
	t.Parallel()
	finding := evaluateD1GitDivergence([]string{
		"feat(TASK-001): commit",
	}, map[string]string{
		"TASK-001": "open",
	})

	assert.Equal(t, SeverityWarning, finding.Severity)
	assert.Contains(t, finding.Items[0], "TASK-001")
}
