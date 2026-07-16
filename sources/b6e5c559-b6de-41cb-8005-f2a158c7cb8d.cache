# E4 Glamour Render Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `render-context` human output from plain text to Glamour-rendered markdown.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S2 section)

**Depends on:** `2026-03-18-e4-s7-traceability.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Glamour v0.8, testify

---

## File Structure

| File | Change |
|---|---|
| `internal/context/render.go` | Replace `RenderHuman` with Glamour-rendered markdown; add `RenderAgent` |
| `internal/context/context_test.go` | Add Glamour render tests |

---

## Tasks

### Task 1: Glamour render upgrade (render-context human format)

**Files:**
- Modify: `internal/context/render.go`
- Modify: `internal/context/context_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/context/context_test.go (append)
func TestRenderHumanContainsLayerContent(t *testing.T) {
	ctx := &Context{
		IssueID: "TSK-1",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: "# My Issue\n\n## Definition of Done\nShip it."},
			{Name: "notes", Priority: 6, Content: "## Notes\n- note one"},
		},
	}
	out := RenderHuman(ctx)
	assert.Contains(t, out, "My Issue")
	assert.Contains(t, out, "Definition of Done")
	assert.Contains(t, out, "note one")
}
```

- [ ] **Step 2: Run test to verify it fails (or passes with plain text)**

```bash
go test ./internal/context/... -run TestRenderHumanContains -v
```

Note: this test may already pass with the current plain-text implementation. The goal is to **upgrade** to Glamour without breaking it.

- [ ] **Step 3: Upgrade RenderHuman to Glamour**

Replace the current `RenderHuman` implementation in `internal/context/render.go`:

```go
package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

// RenderAgent returns a JSON encoding of the context layers.
func RenderAgent(ctx *Context) (string, error) {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal context: %w", err)
	}
	return string(data), nil
}

// RenderHuman returns Glamour-rendered markdown combining all layers.
// Falls back to plain text if Glamour rendering fails.
func RenderHuman(ctx *Context) string {
	md := buildMarkdown(ctx)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return plainText(ctx)
	}
	out, err := renderer.Render(md)
	if err != nil {
		return plainText(ctx)
	}
	return out
}

// buildMarkdown assembles all non-empty layers into a single markdown document.
func buildMarkdown(ctx *Context) string {
	var sb strings.Builder
	for i, layer := range ctx.Layers {
		if layer.Content == "" {
			continue
		}
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(layer.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// plainText is the pre-Glamour fallback.
func plainText(ctx *Context) string {
	var sb strings.Builder
	for i, layer := range ctx.Layers {
		if layer.Content == "" {
			continue
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", layer.Name, layer.Content)
	}
	return sb.String()
}
```

- [ ] **Step 4: Run all context tests**

```bash
go test ./internal/context/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/context/render.go internal/context/context_test.go
git commit -m "feat(context): upgrade render-context human format to Glamour markdown rendering"
```
