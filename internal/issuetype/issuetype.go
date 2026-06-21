package issuetype

// IsValid returns true if t is a known issue type.
func IsValid(t string) bool {
	return validTypes[t]
}

// IsLegalHierarchy returns true if child type is legal under parent type.
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

// IsWorkable returns true if type can be claimed/executed.
func IsWorkable(t string) bool {
	return workable[t]
}

// RequiresAcceptance returns true if type requires explicit acceptance criteria.
func RequiresAcceptance(t string) bool {
	return requiresAcceptance[t]
}

// All returns all valid issue types.
func All() []string {
	return allTypes
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

// workable defines which types can be claimed/executed.
var workable = map[string]bool{
	"task": true,
}

// requiresAcceptance defines which types require explicit acceptance criteria.
var requiresAcceptance = map[string]bool{
	"epic":    true,
	"story":   true,
	"task":    true,
	"bug":     false,
	"feature": false,
}
