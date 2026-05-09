package orchestrate

import (
	"fmt"
	"strings"
)

// AssemblePrompt renders a complete agent prompt from the provided context
// string and Feedback block.
//
// The rendered prompt always contains, in order:
//  1. The raw context block (task description, code, etc.).
//  2. The scope constraint (if any).
//  3. The named-file list (if any).
//  4. A hard "Do not commit" instruction.
//  5. A failed-checks block (if any failed checks are present).
func AssemblePrompt(context string, fb Feedback) string {
	var sb strings.Builder

	// 1. Context block.
	sb.WriteString(context)
	sb.WriteString("\n")

	// 2. Scope constraint (retry ≥ 2).
	if fb.ScopeConstraint != "" {
		sb.WriteString("\n")
		sb.WriteString(fb.ScopeConstraint)
		sb.WriteString("\n")
	}

	// 3. Named file list (retry ≥ 3).
	if len(fb.NamedFiles) > 0 {
		sb.WriteString("\nPermitted files:\n")
		for _, f := range fb.NamedFiles {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
	}

	// 4. Mandatory instruction — always present.
	sb.WriteString("\nDo not commit any changes.\n")

	// 5. Failed checks block — only when there are failures.
	if len(fb.FailedChecks) > 0 {
		sb.WriteString("\nFailed checks:\n")
		for _, c := range fb.FailedChecks {
			fmt.Fprintf(&sb, "  [%s] %s: %s\n", c.Severity, c.Name, c.Message)
		}
	}

	return sb.String()
}
