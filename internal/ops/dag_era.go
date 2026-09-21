package ops

// DAGEra is the decode-on-read classification of a dag-transition payload.
// Historical `arm confirm` ops set Confirmed on TargetID (legacy). Current
// `arm dag transition` ops set IssueID and promote subtree confidence
// (canonical). Decode does not rewrite JSONL: ParseLine must not mutate Payload.
type DAGEra uint8

const (
	DAGEraLegacy DAGEra = iota
	DAGEraCanonical
)

// DAGMode is the classified dag-transition action. RootID is TargetID for
// legacy confirm and Payload.IssueID for canonical promote (they may differ).
type DAGMode struct {
	Era        DAGEra
	RootID     string
	Confidence string
	Confirmed  bool
}

// DecodeDAGMode classifies a dag-transition op. Payload.To is confidence on
// the canonical path only; it is not an era discriminator. Empty canonical To
// defaults to verified. IssueID presence selects canonical even if Confirmed
// is also set.
func DecodeDAGMode(op Op) DAGMode {
	if op.Payload.IssueID != "" {
		confidence := op.Payload.To
		if confidence == "" {
			confidence = "verified"
		}
		return DAGMode{
			Era:        DAGEraCanonical,
			RootID:     op.Payload.IssueID,
			Confidence: confidence,
		}
	}
	return DAGMode{
		Era:       DAGEraLegacy,
		RootID:    op.TargetID,
		Confirmed: op.Payload.Confirmed,
	}
}
