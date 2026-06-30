package ops

// GenerateSchema returns the SCHEMA file content defining positional array format.
func GenerateSchema() string {
	return `# Trellis Op Log Schema v1
#
# Each line is a JSON array: [op_type, target_id, timestamp, worker_id, payload]
#
# Position 0: op_type (string) — one of: create, claim, heartbeat, transition,
#             note, link, unlink, source-link, source-fingerprint, dag-transition, decision, assign
# Position 1: target_id (string) — issue/node/source ID this op targets
# Position 2: timestamp (integer) — Unix epoch seconds
# Position 3: worker_id (string) — UUID of the worker emitting this op
# Position 4: payload (object) — op-type-specific fields (see below)
#
# Forward compatibility: new fields may be appended to the array.
# Readers MUST ignore extra positions. Missing positions get defaults.
#
# Payload fields by op type:
#   create:             title, parent, type, scope, acceptance, definition_of_done,
#                       context, source_citation, priority, estimated_complexity
#   claim:              ttl
#   heartbeat:          (empty object)
#   transition:         to, outcome, branch (optional), pr (optional)
#   note:               msg
#   link:               dep, rel
#   unlink:             dep, rel
#   source-link:        source_id, section, anchor, quote
#   source-fingerprint: sha, version_id, provider
#   dag-transition:     to, uncovered_acknowledged
#   decision:           topic, choice, rationale, affects
#   assign:             assigned_to (empty string to unassign)
`
}
