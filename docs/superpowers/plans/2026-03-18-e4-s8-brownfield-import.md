# E4 Brownfield Import Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement brownfield CSV/JSON import of existing work items and the `confirm` command, with `--format=json` output for agent-friendly use.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S8 section)

**Depends on:** `2026-03-18-e4-s6-dag-summary.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Cobra v1.8, testify

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/importbf/import.go` | CSV/JSON parser, `ImportedIssue` type |
| `internal/importbf/import_test.go` | Parser tests |
| `cmd/trellis/import.go` | `import` command with `--format=json` output |
| `cmd/trellis/confirm.go` | `confirm` command |
| `cmd/trellis/main.go` | Register `import` and `confirm` commands |

---

## Tasks

### Task 1: Brownfield import

**Files:**
- Create: `internal/importbf/import.go` (package name `importbf` to avoid clash with Go builtin `import`)
- Create: `internal/importbf/import_test.go`
- Create: `cmd/trellis/import.go`
- Create: `cmd/trellis/confirm.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/importbf/import_test.go
package importbf_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/importbf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleCSV = `id,title,type,parent,scope
TSK-X1,First import,task,,internal/foo/*.go
TSK-X2,Second import,task,,"internal/bar/*.go,internal/baz/*.go"
`

func TestParseCSV(t *testing.T) {
	items, err := importbf.ParseCSV([]byte(sampleCSV))
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "TSK-X1", items[0].ID)
	assert.Equal(t, "task", items[0].Type)
	assert.Equal(t, []string{"internal/foo/*.go"}, items[0].Scope)
}

var sampleJSON = `[
  {"id":"TSK-J1","title":"JSON import","type":"story","scope":["internal/ops/**"]},
  {"id":"TSK-J2","title":"Another","type":"task","parent":"TSK-J1"}
]`

func TestParseJSON(t *testing.T) {
	items, err := importbf.ParseJSON([]byte(sampleJSON))
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "TSK-J1", items[0].ID)
	assert.Equal(t, "story", items[0].Type)
}

func TestImportedItemsHaveCorrectProvenance(t *testing.T) {
	items, _ := importbf.ParseCSV([]byte(sampleCSV))
	for _, item := range items {
		assert.Equal(t, "imported", item.Provenance.Method)
		assert.Equal(t, "inferred", item.Provenance.Confidence)
		assert.True(t, item.RequiresConfirmation)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/importbf/... -v
```

Expected: FAIL.

- [ ] **Step 3: Implement import parser**

```go
// internal/importbf/import.go
package importbf

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
)

// ImportedIssue represents a work item parsed from a brownfield import file.
type ImportedIssue struct {
	ID                   string
	Title                string
	Type                 string
	Parent               string
	Scope                []string
	Provenance           Provenance
	RequiresConfirmation bool
}

// Provenance describes the origin of an imported issue.
type Provenance struct {
	Method     string `json:"method"`     // always "imported"
	Confidence string `json:"confidence"` // always "inferred"
}

var defaultProvenance = Provenance{Method: "imported", Confidence: "inferred"}

// ParseCSV parses brownfield issues from CSV bytes.
// Expected columns: id, title, type, parent, scope (comma-separated globs in quotes).
func ParseCSV(data []byte) ([]ImportedIssue, error) {
	r := csv.NewReader(bytes.NewReader(data))
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, nil
	}
	// Build column index from header row
	header := rows[0]
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	get := func(row []string, col string) string {
		if i, ok := idx[col]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	var items []ImportedIssue
	for _, row := range rows[1:] {
		scopeStr := get(row, "scope")
		var scope []string
		if scopeStr != "" {
			for _, s := range strings.Split(scopeStr, ",") {
				if t := strings.TrimSpace(s); t != "" {
					scope = append(scope, t)
				}
			}
		}
		items = append(items, ImportedIssue{
			ID:                   get(row, "id"),
			Title:                get(row, "title"),
			Type:                 get(row, "type"),
			Parent:               get(row, "parent"),
			Scope:                scope,
			Provenance:           defaultProvenance,
			RequiresConfirmation: true,
		})
	}
	return items, nil
}

// ParseJSON parses brownfield issues from JSON bytes (array of objects).
func ParseJSON(data []byte) ([]ImportedIssue, error) {
	var raw []struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Type   string   `json:"type"`
		Parent string   `json:"parent"`
		Scope  []string `json:"scope"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	items := make([]ImportedIssue, len(raw))
	for i, r := range raw {
		items[i] = ImportedIssue{
			ID: r.ID, Title: r.Title, Type: r.Type,
			Parent: r.Parent, Scope: r.Scope,
			Provenance:           defaultProvenance,
			RequiresConfirmation: true,
		}
	}
	return items, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/importbf/... -v
```

Expected: PASS.

- [ ] **Step 5: Implement import command**

```go
// cmd/trellis/import.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/importbf"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var (
		sourceID string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import brownfield issues from CSV or JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			issuesDir := appCtx.IssuesDir

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			var items []importbf.ImportedIssue
			ext := strings.ToLower(filepath.Ext(filePath))
			switch ext {
			case ".csv":
				items, err = importbf.ParseCSV(data)
			case ".json":
				items, err = importbf.ParseJSON(data)
			default:
				return fmt.Errorf("unsupported file type %q (expected .csv or .json)", ext)
			}
			if err != nil {
				return fmt.Errorf("parse %s: %w", filePath, err)
			}

			if dryRun {
				for _, item := range items {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "would import: %s %q [%s]\n",
						item.ID, item.Title, item.Type)
				}
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			imported := 0
			var importedIDs []string
			for _, item := range items {
				o := ops.Op{
					Type:      ops.OpCreate,
					TargetID:  item.ID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						Title:    item.Title,
						NodeType: item.Type,
						Parent:   item.Parent,
						Scope:    item.Scope,
					},
				}
				if err := appendLowStakesOp(logPath, o); err != nil {
					return fmt.Errorf("emit create op for %s: %w", item.ID, err)
				}
				if sourceID != "" {
					sl := ops.Op{
						Type:      ops.OpSourceLink,
						TargetID:  item.ID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{SourceID: sourceID},
					}
					_ = appendLowStakesOp(logPath, sl)
				}
				importedIDs = append(importedIDs, item.ID)
				imported++
			}

			// JSON output path for agent/CI use
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				type importResult struct {
					Created  int      `json:"created"`
					IssueIDs []string `json:"issue_ids"`
				}
				b, _ := json.Marshal(importResult{Created: imported, IssueIDs: importedIDs})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "imported %d issue(s)\n", imported)
			}
			_ = issuesDir
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceID, "source", "", "Source ID to attach to imported issues")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be imported without writing ops")
	return cmd
}
```

- [ ] **Step 6: Implement confirm command**

```go
// cmd/trellis/confirm.go
package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newConfirmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "confirm <node-id>",
		Short: "Promote an inferred node from draft to verified confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID := args[0]
			issuesDir := appCtx.IssuesDir

			// Verify node exists
			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}
			if _, ok := state.Issues[nodeID]; !ok {
				return fmt.Errorf("node %q not found", nodeID)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			o := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  nodeID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
			}
			if err := appendLowStakesOp(logPath, o); err != nil {
				return fmt.Errorf("emit dag-transition op: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "confirmed %s (inferred → verified)\n", nodeID)
			return nil
		},
	}
}
```

- [ ] **Step 7: Register new commands in main.go**

```go
rootCmd.AddCommand(newImportCmd())
rootCmd.AddCommand(newConfirmCmd())
```

- [ ] **Step 8: Build and verify**

```bash
go build ./cmd/trellis/... && ./bin/arm import --help && ./bin/arm confirm --help
```

Expected: both show help without errors.

- [ ] **Step 9: Commit**

```bash
git add internal/importbf/ cmd/trellis/import.go cmd/trellis/confirm.go cmd/trellis/main.go
git commit -m "feat(import): brownfield CSV/JSON import and confirm command (E3-S8)"
```
