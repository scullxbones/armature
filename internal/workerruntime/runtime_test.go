package workerruntime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPolicyAndStatesExposeV1RuntimeSurface(t *testing.T) {
	policy := DefaultPolicy()

	assert.Equal(t, StateIdle, InitialState())
	assert.Equal(t, StateClaimPending, NextState(StatePolling, TriggerClaimSelected))
	assert.Equal(t, "execution_lane_routing", policy.Subworkflows.ExecutionLaneRouting.Workflow)
	assert.False(t, policy.Subworkflows.ExecutionLaneRouting.ExceptionAgentEnabled)
	assert.Equal(t, 1, policy.Retry.MaxVerificationFailures)
	assert.Equal(t, 30*time.Second, policy.Cooldown.NoReadyWorkDelay)
}
