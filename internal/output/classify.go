package output

// Channel is a command's Agent Output Contract classification.
// Every command is agent-facing or Protocol Output; the default is agent-facing.
type Channel string

const (
	ChannelAgentFacing    Channel = "agent-facing"
	ChannelProtocolOutput Channel = "protocol-output"
)

// ChannelAnnotationKey is the cobra.Command.Annotations key that stores Channel.
// Absence, emptiness, or an unrecognized value means agent-facing (N8).
const ChannelAnnotationKey = "armature.output.channel"

// Classify returns the command's output channel from its annotations.
// A new command cannot become Protocol Output by omitting the annotation.
func Classify(annotations map[string]string) Channel {
	if Channel(annotations[ChannelAnnotationKey]) == ChannelProtocolOutput {
		return ChannelProtocolOutput
	}
	return ChannelAgentFacing
}

// MarkProtocolOutput returns a copy of annotations with an explicit Protocol Output classification.
func MarkProtocolOutput(annotations map[string]string) map[string]string {
	out := make(map[string]string, len(annotations)+1)
	for k, v := range annotations {
		out[k] = v
	}
	out[ChannelAnnotationKey] = string(ChannelProtocolOutput)
	return out
}
