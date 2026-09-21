package ops

import "strings"

// DecodeScope splits legacy comma-joined scope entries at the load boundary.
// Historical create/amend payloads stored multiple paths as one ", "-joined
// string; materialized state always holds one path per element. Decode is
// on-read: ParseLine must not mutate Payload.Scope.
func DecodeScope(scope []string) []string {
	result := make([]string, 0, len(scope))
	for _, entry := range scope {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, ", ") {
			for part := range strings.SplitSeq(entry, ", ") {
				if part = strings.TrimSpace(part); part != "" {
					result = append(result, part)
				}
			}
		} else if entry = strings.TrimSpace(entry); entry != "" {
			result = append(result, entry)
		}
	}
	return result
}
