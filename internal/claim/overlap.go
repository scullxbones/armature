package claim

import "path/filepath"

// ScopesOverlap checks if two scope glob lists have any overlap.
func ScopesOverlap(scopeA, scopeB []string) bool {
	for _, a := range scopeA {
		for _, b := range scopeB {
			if globOverlaps(a, b) {
				return true
			}
		}
	}
	return false
}

func globOverlaps(a, b string) bool {
	if matched, _ := filepath.Match(a, b); matched {
		return true
	}
	if matched, _ := filepath.Match(b, a); matched {
		return true
	}
	dirA := extractDir(a)
	dirB := extractDir(b)
	if dirA == "" || dirB == "" {
		return false
	}
	return hasPrefix(dirA, dirB) || hasPrefix(dirB, dirA)
}

func extractDir(pattern string) string {
	for i := len(pattern) - 1; i >= 0; i-- {
		if pattern[i] == '/' {
			return pattern[:i]
		}
	}
	return ""
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
