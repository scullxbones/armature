package ops

type dagEra uint8

const (
	dagEraLegacy dagEra = iota
	dagEraCanonical
)

// DAGMode is the classified dag-transition action. RootID is TargetID for
// legacy confirm and Payload.IssueID for canonical promote (they may differ).
type DAGMode struct {
	era        dagEra
	RootID     string
	Confidence string
	Confirmed  bool
}

func (m DAGMode) Canonical() bool {
	return m.era == dagEraCanonical
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
			era:        dagEraCanonical,
			RootID:     op.Payload.IssueID,
			Confidence: confidence,
		}
	}
	return DAGMode{
		era:       dagEraLegacy,
		RootID:    op.TargetID,
		Confirmed: op.Payload.Confirmed,
	}
}
