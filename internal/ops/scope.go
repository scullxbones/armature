package ops

import "strings"

// DecodeScope splits comma-joined scope entries at the load boundary.
// Historical create/amend payloads stored multiple paths as one comma-joined
// string (with or without spaces); materialized state always holds one path
// per element. Decode is on-read: ParseLine must not mutate Payload.Scope.
func DecodeScope(scope []string) []string {
	result := make([]string, 0, len(scope))
	for _, entry := range scope {
		for part := range strings.SplitSeq(entry, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
