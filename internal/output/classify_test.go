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

func TestArtifactOutputClassificationIsModeSensitive_REQ_AOC_S1_T3(t *testing.T) {
	t.Parallel()

	reviewPrep := MarkArtifactOutput(nil, ArtifactMode{
		Citation:          CitationReviewBundleSchema,
		WhenAllFlagsUnset: []string{"output"},
	})
	require.Equal(t, ChannelArtifactOutput, Classify(reviewPrep),
		"review prepare with --output unset is Artifact Output")
	require.Equal(t, ChannelAgentFacing, ClassifyFlags(reviewPrep, map[string]bool{"output": true}),
		"review prepare --output <file> stays agent-facing")

	completion := MarkArtifactOutput(nil, ArtifactMode{Citation: CitationShellCompletionGrammar})
	require.Equal(t, ChannelArtifactOutput, Classify(completion))
	require.Equal(t, ChannelArtifactOutput, ClassifyFlags(completion, map[string]bool{"format": true}),
		"completion has no result mode; every invocation is Artifact Output")

	dagApply := MarkArtifactOutput(nil, ArtifactMode{
		Citation:       CitationPlanSchema,
		WhenAnyFlagSet: []string{"schema", "example"},
	})
	require.Equal(t, ChannelAgentFacing, Classify(dagApply),
		"dag apply without --schema/--example is agent-facing")
	require.Equal(t, ChannelArtifactOutput, ClassifyFlags(dagApply, map[string]bool{"schema": true}))
	require.Equal(t, ChannelArtifactOutput, ClassifyFlags(dagApply, map[string]bool{"example": true}))
	require.Equal(t, ChannelAgentFacing, ClassifyFlags(dagApply, map[string]bool{"dry-run": true}),
		"unrelated flags do not opt dag apply out")
}

func TestArtifactOutputModesCiteGoverningShape_REQ_AOC_S1_T3(t *testing.T) {
	t.Parallel()

	uncited := MarkArtifactOutput(nil, ArtifactMode{})
	require.Equal(t, ChannelAgentFacing, Classify(uncited),
		"an Artifact Output mark without a governing-shape citation is not admissible")

	reviewPrep := MarkArtifactOutput(nil, ArtifactMode{
		Citation:          CitationReviewBundleSchema,
		WhenAllFlagsUnset: []string{"output"},
	})
	require.Equal(t, CitationReviewBundleSchema, reviewPrep[ArtifactCitationKey])
	require.Equal(t, ChannelArtifactOutput, Classify(reviewPrep))

	completion := MarkArtifactOutput(nil, ArtifactMode{Citation: CitationShellCompletionGrammar})
	require.Equal(t, CitationShellCompletionGrammar, completion[ArtifactCitationKey])

	dagApply := MarkArtifactOutput(nil, ArtifactMode{
		Citation:       CitationPlanSchema,
		WhenAnyFlagSet: []string{"schema", "example"},
	})
	require.Equal(t, CitationPlanSchema, dagApply[ArtifactCitationKey])
	require.Equal(t, "schema,example", dagApply[ArtifactWhenAnyFlagSetKey])
}

func TestArtifactCommandsKeepResultModesAgentFacing_REQ_AOC_S1_T3(t *testing.T) {
	t.Parallel()

	reviewPrep := MarkArtifactOutput(map[string]string{"keep": "yes"}, ArtifactMode{
		Citation:          CitationReviewBundleSchema,
		WhenAllFlagsUnset: []string{"output"},
	})
	require.Equal(t, "yes", reviewPrep["keep"])
	require.Equal(t, ChannelAgentFacing, ClassifyFlags(reviewPrep, map[string]bool{"output": true}))

	dagApply := MarkArtifactOutput(nil, ArtifactMode{
		Citation:       CitationPlanSchema,
		WhenAnyFlagSet: []string{"schema", "example"},
	})
	require.Equal(t, ChannelAgentFacing, ClassifyFlags(dagApply, nil))
	require.Equal(t, ChannelAgentFacing, ClassifyFlags(dagApply, map[string]bool{"plan": true}))
	require.Equal(t, ChannelProtocolOutput, Classify(MarkProtocolOutput(nil)),
		"Protocol Output remains a distinct explicit class")
}
