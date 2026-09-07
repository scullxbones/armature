package output

import "strings"

// Channel is a command's Agent Output Contract classification.
// Every command mode is agent-facing, Protocol Output, or Artifact Output;
// the default is agent-facing.
type Channel string

const (
	ChannelAgentFacing    Channel = "agent-facing"
	ChannelProtocolOutput Channel = "protocol-output"
	ChannelArtifactOutput Channel = "artifact-output"
)

// ChannelAnnotationKey is the cobra.Command.Annotations key that stores Channel.
// Absence, emptiness, or an unrecognized value means agent-facing (N9).
const ChannelAnnotationKey = "armature.output.channel"

// Artifact citation and mode-selector annotation keys (N8 / N9.4–N9.5).
const (
	ArtifactCitationKey          = "armature.output.artifact.citation"
	ArtifactWhenAnyFlagSetKey    = "armature.output.artifact.when-any-flag-set"
	ArtifactWhenAllFlagsUnsetKey = "armature.output.artifact.when-all-flags-unset"
)

// Governing-shape citations for the three normative Artifact Output modes (N8).
const (
	CitationReviewBundleSchema     = "docs/schemas/review-bundle.schema.json"
	CitationShellCompletionGrammar = "shell completion-script grammar (bash/zsh/fish/powershell)"
	CitationPlanSchema             = "docs/schemas/plan.schema.json"
)

// ArtifactMode selects Artifact Output for a command or for particular flags.
// Citation is required (N8.2); a mark without a citation is not admissible.
// WhenAnyFlagSet: artifact if any named flag is set (dag apply --schema|--example).
// WhenAllFlagsUnset: artifact if every named flag is unset (review prepare without --output).
// Both empty: every mode of the command is Artifact Output (completion).
type ArtifactMode struct {
	Citation          string
	WhenAnyFlagSet    []string
	WhenAllFlagsUnset []string
}

// Classify returns the command's output channel from its annotations,
// treating all flags as unset. A new command cannot opt out by omission.
func Classify(annotations map[string]string) Channel {
	return ClassifyFlags(annotations, nil)
}

// ClassifyFlags returns the output channel for an invocation whose set flags
// are the keys of setFlags with true values. Unlisted flags are unset.
func ClassifyFlags(annotations map[string]string, setFlags map[string]bool) Channel {
	if Channel(annotations[ChannelAnnotationKey]) == ChannelProtocolOutput {
		return ChannelProtocolOutput
	}
	if Channel(annotations[ChannelAnnotationKey]) != ChannelArtifactOutput {
		return ChannelAgentFacing
	}
	if strings.TrimSpace(annotations[ArtifactCitationKey]) == "" {
		return ChannelAgentFacing
	}
	anySet := splitCSV(annotations[ArtifactWhenAnyFlagSetKey])
	allUnset := splitCSV(annotations[ArtifactWhenAllFlagsUnsetKey])
	if len(anySet) == 0 && len(allUnset) == 0 {
		return ChannelArtifactOutput
	}
	for _, name := range anySet {
		if setFlags[name] {
			return ChannelArtifactOutput
		}
	}
	if len(allUnset) > 0 {
		for _, name := range allUnset {
			if setFlags[name] {
				return ChannelAgentFacing
			}
		}
		return ChannelArtifactOutput
	}
	return ChannelAgentFacing
}

// MarkProtocolOutput returns a copy of annotations with an explicit Protocol Output classification.
func MarkProtocolOutput(annotations map[string]string) map[string]string {
	out := cloneAnnotations(annotations, 1)
	out[ChannelAnnotationKey] = string(ChannelProtocolOutput)
	return out
}

// MarkArtifactOutput returns a copy of annotations with an explicit, cited Artifact Output classification.
func MarkArtifactOutput(annotations map[string]string, mode ArtifactMode) map[string]string {
	out := cloneAnnotations(annotations, 4)
	out[ChannelAnnotationKey] = string(ChannelArtifactOutput)
	out[ArtifactCitationKey] = mode.Citation
	if len(mode.WhenAnyFlagSet) > 0 {
		out[ArtifactWhenAnyFlagSetKey] = strings.Join(mode.WhenAnyFlagSet, ",")
	}
	if len(mode.WhenAllFlagsUnset) > 0 {
		out[ArtifactWhenAllFlagsUnsetKey] = strings.Join(mode.WhenAllFlagsUnset, ",")
	}
	return out
}

func cloneAnnotations(annotations map[string]string, extra int) map[string]string {
	out := make(map[string]string, len(annotations)+extra)
	for k, v := range annotations {
		out[k] = v
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
