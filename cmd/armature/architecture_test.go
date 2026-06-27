package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6 verifies that handler files
// do not directly call materialize functions or build state paths with filepath.Join.
// Exemptions:
// - materialize.go (the arm-materialize command)
// - render_context.go (time-travel branch uses MaterializeAtSHA)
// - claim.go, transition.go (already migrated)
func TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6(t *testing.T) {
	// Scope of files that must be migrated
	scope := []string{
		"create.go", "assign.go", "dagsum.go", "confirm.go", "list.go",
		"context_history.go", "decompose.go", "harness_context.go", "hook.go",
		"scope_delete.go", "merged.go", "scope_rename.go", "ready.go",
		"reparent.go", "show.go", "stalereview.go", "sync.go", "tui.go", "validate.go",
	}

	// Exempt files
	exempt := map[string]bool{
		"render_context.go": true,
		"claim.go":          true,
		"transition.go":     true,
		"materialize.go":    true,
	}

	fset := token.NewFileSet()
	violations := []string{}

	// Find the cmd/armature directory by using runtime to locate this test file
	_, thisTestFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisTestFile)

	for _, filename := range scope {
		if exempt[filename] {
			continue
		}

		path := filepath.Join(baseDir, filename)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}

		file, err := parser.ParseFile(fset, path, src, parser.AllErrors)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", path, err)
		}

		// Check for direct materialize calls and state path construction
		checkFile(path, file, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("handlers must use newSnapshotStore(ctx) instead of direct materialize calls:\n%s",
			strings.Join(violations, "\n"))
	}
}

// checkFile walks the AST to find violations
func checkFile(filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for direct materialize.* calls
		if isMaterializeCall(node) {
			pos := node.Pos()
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct materialize call detected", filename, pos))
		}

		// Check for filepath.Join(ctx.StateDir, ...) pattern
		if isStatePathJoin(node) {
			pos := node.Pos()
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct state path construction with filepath.Join", filename, pos))
		}
		return true
	})
}

// isMaterializeCall detects calls to materialize.Materialize*, materialize.LoadIssue, or materialize.LoadIndex
// Does NOT match materialize.MaterializeAtSHA (which is exempt for time-travel)
func isMaterializeCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Must be a method on materialize package
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "materialize" {
		return false
	}

	// Check for forbidden calls
	methodName := sel.Sel.Name
	forbidden := map[string]bool{
		"Materialize":               true,
		"MaterializeAndReturn":      true,
		"MaterializeAndReturnQuiet": true,
		"LoadIssue":                 true,
		"LoadIndex":                 true,
	}

	// MaterializeAtSHA is exempt (time-travel)
	if methodName == "MaterializeAtSHA" {
		return false
	}

	return forbidden[methodName]
}

// isStatePathJoin detects filepath.Join(ctx.StateDir, ...) pattern
func isStatePathJoin(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Must be filepath.Join
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "filepath" || sel.Sel.Name != "Join" {
		return false
	}

	// Check if first argument is ctx.StateDir
	if len(call.Args) == 0 {
		return false
	}

	firstArg, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok {
		return false
	}

	ctx, ok := firstArg.X.(*ast.Ident)
	if !ok || ctx.Name != "ctx" || firstArg.Sel.Name != "StateDir" {
		return false
	}

	return true
}
