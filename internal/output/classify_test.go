package output

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandClassificationDefaultsToAgentFacing_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	require.Equal(t, ChannelAgentFacing, Classify(nil), "nil annotations must default to agent-facing")
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{}), "empty annotations must default to agent-facing")
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{"other": "x"}), "unrelated annotations must default to agent-facing")
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{ChannelAnnotationKey: ""}), "empty channel must not opt out")
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{ChannelAnnotationKey: "protocol"}), "unknown channel must not opt out")
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{ChannelAnnotationKey: string(ChannelAgentFacing)}))

	marked := MarkProtocolOutput(nil)
	require.Equal(t, ChannelProtocolOutput, Classify(marked), "Protocol Output requires an explicit mark")

	merged := MarkProtocolOutput(map[string]string{"keep": "yes"})
	require.Equal(t, "yes", merged["keep"])
	require.Equal(t, ChannelProtocolOutput, Classify(merged))
	require.Equal(t, ChannelAgentFacing, Classify(map[string]string{"keep": "yes"}), "omission on a new command stays agent-facing")
}
