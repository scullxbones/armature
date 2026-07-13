// Package issuetype defines the set of issue types (epic, story, task, bug, ADR, etc.) and their validation rules.
package issuetype

// IsValid returns true if t is a known issue type.
func IsValid(t string) bool {
	return validTypes[t]
}

// IsLegalHierarchy returns true if child type is legal under parent type.
//
// Full set of permitted parent→child relationships:
//
//	epic    → story, feature, task, bug
//	story   → feature, task, bug
//	feature → task, bug
//	task    → (nothing)
//	bug     → (nothing)
//
// Changes from the previous inline validHierarchy switch in validate.go:
//   - epic→feature is now permitted (previously epic only allowed story, task, bug)
//   - story→feature is now permitted to match the planned vocabulary
//   - feature as a parent type now explicitly permits task and bug children
//     (previously feature fell into the default: return true branch, making
//     feature→anything silently pass, including illegal combinations)
//   - unknown parent or child types now return false (hardening; previously an
//     unknown parent type fell through to default: return true)
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
// Ready-eligible types are task, feature, story, and bug — types that can be actively worked on.
func IsReadyEligible(t string) bool {
	return readyEligible[t]
}

// RequiredFields returns the canonical required field names for typ.
func RequiredFields(typ string) []string {
	fields := requiredFields[typ]
	if len(fields) == 0 {
		return nil
	}
	return append([]string(nil), fields...)
}

// All returns all valid issue types in declaration order (epic, story, feature, task, bug).
// Returns a defensive copy so callers cannot mutate the canonical list.
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

// allTypes is the ordered list of valid types (by hierarchy level, not alphabetically).
var allTypes = []string{"epic", "story", "feature", "task", "bug"}

// hierarchy defines which parent types may contain which child types.
var hierarchy = map[string]map[string]bool{
	"epic":    {"story": true, "feature": true, "task": true, "bug": true},
	"story":   {"feature": true, "task": true, "bug": true},
	"feature": {"task": true, "bug": true},
	"task":    {},
	"bug":     {},
}

// readyEligible defines which types can appear in the ready queue.
var readyEligible = map[string]bool{
	"task":    true,
	"feature": true,
	"story":   true,
	"bug":     true,
}

// requiredFields defines the canonical required fields for each issue type.
var requiredFields = map[string][]string{
	"task": {"scope", "acceptance", "definition_of_done"},
}
