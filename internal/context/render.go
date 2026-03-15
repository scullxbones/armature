package context

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderAgent returns a JSON encoding of the context layers.
func RenderAgent(ctx *Context) (string, error) {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal context: %w", err)
	}
	return string(data), nil
}

// RenderHuman returns plain text with layer headers and content.
func RenderHuman(ctx *Context) string {
	var sb strings.Builder
	for i, layer := range ctx.Layers {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", layer.Name, layer.Content)
	}
	return sb.String()
}
