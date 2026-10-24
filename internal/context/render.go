package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

// RenderAgent returns working memory as agent JSON: assemble, optional
// truncation, then indented JSON. Callers pass IssueFacts and Options; file
// reading stays inside the package.
func RenderAgent(issueID string, issues map[string]IssueFacts, opts Options) (string, error) {
	mem, err := assemble(issueID, issues, opts.reader())
	if err != nil {
		return "", err
	}
	if !opts.SkipTruncate {
		mem = truncate(mem, opts.TokenBudget)
	}
	return marshalAgent(mem)
}

// RenderHuman returns working memory as Glamour-rendered text, falling back
// to plain text if Glamour fails. Assemble, optional truncation, and file
// reading stay inside the package.
func RenderHuman(issueID string, issues map[string]IssueFacts, opts Options) (string, error) {
	mem, err := assemble(issueID, issues, opts.reader())
	if err != nil {
		return "", err
	}
	if !opts.SkipTruncate {
		mem = truncate(mem, opts.TokenBudget)
	}
	return formatHuman(mem), nil
}

func marshalAgent(mem *workingMemory) (string, error) {
	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal context: %w", err)
	}
	return string(data), nil
}

func formatHuman(mem *workingMemory) string {
	var sb strings.Builder
	for i, item := range mem.Layers {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", item.Name, item.Content)
	}
	plain := sb.String()

	rendered, err := glamour.Render(plain, "auto")
	if err != nil {
		return plain
	}
	return rendered
}
