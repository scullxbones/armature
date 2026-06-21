package issuetype

// IsValid returns true if t is a known issue type.
func IsValid(t string) bool {
	return validTypes[t]
}

// IsLegalHierarchy returns true if child type is legal under parent type.
//
// Semantics vs. the previous inline switch in validate.go:
//   - epic may now parent feature (intentional: feature is a first-class type)
//   - unknown parent or child types return false (intentional hardening; previously unknown parents passed via default: return true)
func IsLegalHierarchy(parent, child string) bool {
	if _, ok := validTypes[parent]; !ok {
		return false
	}
	if _, ok := validTypes[child]; !ok {
		return false
	}
	allowed, ok := hierarchy[parent]
	return ok && allowed[child]
}

// IsReadyEligible returns true if the type can appear in the ready queue.
// Ready-eligible types are task, feature, and story — types that can be actively worked on.
func IsReadyEligible(t string) bool {
	return readyEligible[t]
}

// All returns all valid issue types. Returns a defensive copy so callers cannot mutate the canonical list.
func All() []string {
	return append([]string(nil), allTypes...)
}

// validTypes is the complete set of accepted issue types.
var validTypes = map[string]bool{
	"epic":    true,
	"story":   true,
	"feature": true,
	"task":    true,
	"bug":     true,
}

// allTypes is the sorted list of valid types.
var allTypes = []string{"epic", "story", "feature", "task", "bug"}

// hierarchy defines which parent types may contain which child types.
var hierarchy = map[string]map[string]bool{
	"epic":    {"story": true, "feature": true, "task": true, "bug": true},
	"story":   {"task": true, "bug": true},
	"feature": {"task": true, "bug": true},
	"task":    {},
	"bug":     {},
}

// readyEligible defines which types can appear in the ready queue.
var readyEligible = map[string]bool{
	"task":    true,
	"feature": true,
	"story":   true,
}
