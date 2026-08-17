package ops

import "slices"

// classifiedValidity is the exhaustive AffectsValidity census.
// Every materialize.RegisteredOpTypes() entry must appear here; an
// unclassified new type fails CI via TestAffectsValidityCensus.
var classifiedValidity = map[string]bool{
	OpCreate:             true,
	OpAmend:              true,
	OpLink:               true,
	OpUnlink:             true,
	OpReparent:           true,
	OpSourceLink:         true,
	OpDecision:           true,
	OpTransition:         true,
	OpDAGTransition:      true,
	OpScopeRename:        true,
	OpScopeDelete:        true,
	OpCitationAccepted:   true,
	OpClaim:              false,
	OpHeartbeat:          false,
	OpNote:               false,
	OpNoteDelete:         false,
	OpAssign:             false,
	OpSourceFingerprint:  false,
	OpGateEvidence:       false,
	OpAssessmentAttested: false,
}

// AffectsValidity reports whether appending this op type can change what
// arm validate reports. Unclassified types return false; the census test
// fails on the omission.
func AffectsValidity(opType string) bool {
	return classifiedValidity[opType]
}

// ClassifiedValidity reports the census entry for opType. classified is
// false when the type is missing from the map.
func ClassifiedValidity(opType string) (affects bool, classified bool) {
	affects, classified = classifiedValidity[opType]
	return affects, classified
}

// ClassifiedOpTypes returns every op type present in the AffectsValidity census.
func ClassifiedOpTypes() []string {
	types := make([]string, 0, len(classifiedValidity))
	for opType := range classifiedValidity {
		types = append(types, opType)
	}
	slices.Sort(types)
	return types
}
