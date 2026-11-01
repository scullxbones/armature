package adapters

import (
	"fmt"
	"strings"
)

// exactRefFromPatternOutput scans ls-remote / for-each-ref pattern output and
// returns the object name only when some line's refname equals want exactly.
//
// Git ls-remote patterns match the trailing path of a refname (documented
// tail-match), so non-empty stdout alone is not existence. for-each-ref
// listing callers that need an exact hit must use this helper too — do not
// treat the first line as a match.
//
// Line shapes accepted:
//   - "sha refname" / "sha\trefname" (ls-remote)
//   - "refname" (for-each-ref --format=%(refname)) — object returned empty
//   - "sha … refname" (object first, refname last)
func exactRefFromPatternOutput(out, want string) (object string, found bool, err error) {
	want = strings.TrimSpace(want)
	if want == "" {
		return "", false, fmt.Errorf("exact ref match: empty want")
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		var obj, refname string
		switch len(fields) {
		case 1:
			refname = fields[0]
		default:
			obj = fields[0]
			refname = fields[len(fields)-1]
		}
		if refname == "" {
			return "", false, fmt.Errorf("exact ref match: malformed line %q", line)
		}
		if refname == want {
			return obj, true, nil
		}
	}
	return "", false, nil
}

// refsUnderPrefix filters for-each-ref / ls-remote lines to refnames that are
// exactly prefix or under prefix+"/". Rejects trailing-path pattern peers that
// merely end with the same suffix.
func refsUnderPrefix(out, prefix string) ([]string, error) {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return nil, fmt.Errorf("refs under prefix: empty prefix")
	}
	child := prefix + "/"
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		refname := fields[len(fields)-1]
		if refname == "" {
			return nil, fmt.Errorf("refs under prefix: malformed line %q", line)
		}
		if refname == prefix || strings.HasPrefix(refname, child) {
			refs = append(refs, refname)
		}
	}
	return refs, nil
}
