package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWorkerID_ErrorNamesArmWorkerInit_REQ_BUG_1783480206(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)

	_, err := GetWorkerID(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arm worker-init")
	assert.NotContains(t, err.Error(), "trls")
}
