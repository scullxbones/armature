package ops

import (
	"fmt"
	"strings"
)

type schemaOpDoc struct {
	OpType       string
	PayloadField string
}

var schemaOpDocs = []schemaOpDoc{
	{OpType: OpCreate, PayloadField: "title, parent, type, scope, context_files, acceptance, definition_of_done,"},
	{OpType: OpCreate, PayloadField: "                    clear_context_files, context, source_citation, priority,"},
	{OpType: OpCreate, PayloadField: "                    estimated_complexity, confidence, preferred_model"},
	{OpType: OpClaim, PayloadField: "ttl, worktree_path (optional), claim_token"},
	{OpType: OpHeartbeat, PayloadField: "(empty object)"},
	{OpType: OpTransition, PayloadField: "to, outcome, branch (optional), pr (optional),"},
	{OpType: OpTransition, PayloadField: "                       skipped_delivery_gate (optional)"},
	{OpType: OpTransition, PayloadField: "                       restore_claim snapshot fields (rollback only)"},
	{OpType: OpTransition, PayloadField: "                       if_claim_token (conditional compensating rollback only)"},
	{OpType: OpNote, PayloadField: "msg, note_id"},
	{OpType: OpNoteDelete, PayloadField: "note_id"},
	{OpType: OpLink, PayloadField: "dep, rel"},
	{OpType: OpUnlink, PayloadField: "dep, rel"},
	{OpType: OpSourceLink, PayloadField: "source_id, source_url, section, anchor, quote, title"},
	{OpType: OpSourceFingerprint, PayloadField: "sha, version_id, provider"},
	{OpType: OpDAGTransition, PayloadField: "to, issue_id, confirmed, confirmed_noninteractively, uncovered_acknowledged"},
	{OpType: OpDecision, PayloadField: "topic, choice, rationale, affects"},
	{OpType: OpAssign, PayloadField: "assigned_to (empty string to unassign)"},
	{OpType: OpAmend, PayloadField: "type, scope, context_files, clear_context_files, acceptance, definition_of_done"},
	{OpType: OpCitationAccepted, PayloadField: "source_entry_id, confirmed_noninteractively"},
	{OpType: OpScopeRename, PayloadField: "old_path, new_path"},
	{OpType: OpScopeDelete, PayloadField: "deleted_path"},
	{OpType: OpReparent, PayloadField: "parent"},
	{OpType: OpAssessmentAttested, PayloadField: "assessment"},
	{OpType: OpGateEvidence, PayloadField: "profile, command, head_sha, start, end, exit, uncommitted,"},
	{OpType: OpGateEvidence, PayloadField: "                    output_hash, output_head, output_tail, log_path"},
}

// SchemaDocumentedOpTypes returns the ordered op types documented in the schema.
func SchemaDocumentedOpTypes() []string {
	seen := make(map[string]bool, len(schemaOpDocs))
	types := make([]string, 0, len(schemaOpDocs))
	for _, doc := range schemaOpDocs {
		if seen[doc.OpType] {
			continue
		}
		seen[doc.OpType] = true
		types = append(types, doc.OpType)
	}
	return types
}

// GenerateSchema returns the SCHEMA file content defining positional array format.
func GenerateSchema() string {
	var b strings.Builder

	b.WriteString("# Trellis Op Log Schema v1\n")
	b.WriteString("#\n")
	b.WriteString("# Each line is a JSON array: [op_type, target_id, timestamp, worker_id, payload]\n")
	b.WriteString("#\n")
	b.WriteString("# Position 0: op_type (string) - one of: ")
	b.WriteString(strings.Join(SchemaDocumentedOpTypes(), ", "))
	b.WriteString("\n")
	b.WriteString("# Position 1: target_id (string) - issue/node/source ID this op targets\n")
	b.WriteString("# Position 2: timestamp (integer) - Unix epoch seconds\n")
	b.WriteString("# Position 3: worker_id (string) - UUID of the worker emitting this op\n")
	b.WriteString("# Position 4: payload (object) - op-type-specific fields (see below)\n")
	b.WriteString("#\n")
	b.WriteString("# Forward compatibility: new fields may be appended to the array.\n")
	b.WriteString("# Readers MUST ignore extra positions. Missing positions get defaults.\n")
	b.WriteString("#\n")
	b.WriteString("# Payload fields by op type:\n")

	for _, doc := range schemaOpDocs {
		if strings.HasPrefix(doc.PayloadField, "                    ") {
			_, _ = fmt.Fprintf(&b, "# %s\n", doc.PayloadField)
			continue
		}
		_, _ = fmt.Fprintf(&b, "#   %-18s %s\n", doc.OpType+":", doc.PayloadField)
	}

	return b.String()
}
