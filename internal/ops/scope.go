package ops

import "strings"

// DecodeScope trims empty and whitespace-only scope entries at the load
// boundary. Each array element is one path or glob; commas inside an entry are
// part of the path. Historical comma-joined entries are not split.
func DecodeScope(scope []string) []string {
	result := make([]string, 0, len(scope))
	for _, entry := range scope {
		if entry = strings.TrimSpace(entry); entry != "" {
			result = append(result, entry)
		}
	}
	return result
}

// DecodeContextFiles trims empty and whitespace-only entries at the load
// boundary without splitting on commas. context_files is already an array of
// Git paths, so a comma inside an entry is part of the path (e.g. docs/design,v2.md).
func DecodeContextFiles(files []string) []string {
	result := make([]string, 0, len(files))
	for _, entry := range files {
		if entry = strings.TrimSpace(entry); entry != "" {
			result = append(result, entry)
		}
	}
	return result
}
