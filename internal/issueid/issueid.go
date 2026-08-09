// Package issueid defines the durable identifier boundary for Armature issues.
package issueid

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Validate rejects IDs that could become filesystem paths. It deliberately
// does not impose a narrow naming grammar so existing non-path-shaped IDs keep
// their replay compatibility.
func Validate(id string) error {
	if id == "" {
		return fmt.Errorf("issue ID is required")
	}
	if filepath.IsAbs(id) {
		return fmt.Errorf("issue ID %q must not be an absolute path", id)
	}
	// Reject both slash forms on every platform: durable ops can be replayed on
	// a different OS, where the other separator may be path-significant.
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("issue ID %q must not contain path separators", id)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("issue ID %q must not be a path component", id)
	}
	return nil
}
