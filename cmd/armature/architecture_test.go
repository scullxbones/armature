package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6 verifies that handler files
// do not directly call materialize functions or build state paths with filepath.Join.
// Exemptions:
// - materialize.go (the arm-materialize command)
// - render_context.go (time-travel branch uses MaterializeAtSHA)
// - claim.go (already migrated)
//
// ARCHITECTURE GUARD LIMITATION: This test catches direct materialize.* call expressions
// and filepath.Join(x.StateDir, ...) patterns. It does NOT detect the store.Load()-before-append
// anti-pattern, where a handler calls store.Load() purely to read the index before writing an
// op (instead of using store.ReadIndex()). That premature-rematerialization bug class must be
// caught by code review. Files known to be fully migrated to store.ReadIndex() are removed from
// the exempt list and added to scope so the AST guard continues to protect them.
func TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6(t *testing.T) {
	// Scope of files that must be migrated (includes fully-migrated files so the guard
	// actively protects them against regressions).
	scope := []string{
		"create.go", "assign.go", "dagsum.go", "confirm.go", "list.go",
		"context_history.go", "decompose.go", "harness_context.go", "hook.go",
		"scope_delete.go", "merged.go", "scope_rename.go", "ready.go",
		"reparent.go", "show.go", "stalereview.go", "sync.go", "tui.go", "validate.go",
		"transition.go",
	}

	// Exempt files
	exempt := map[string]bool{
		"render_context.go": true,
		"claim.go":          true,
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
		checkFile(fset, path, file, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("handlers must use newSnapshotStore(ctx) instead of direct materialize calls:\n%s",
			strings.Join(violations, "\n"))
	}
}

// checkFile walks the AST to find violations
func checkFile(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for direct materialize.* calls
		if isMaterializeCall(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct materialize call detected", filename, line))
		}

		// Check for filepath.Join(x.StateDir, ...) pattern
		if isStatePathJoin(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct state path construction with filepath.Join", filename, line))
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

// isStatePathJoin detects filepath.Join(x.StateDir, ...) pattern regardless of what x is named.
// This catches ctx.StateDir, appCtx.StateDir, state.ctx.StateDir, and similar expressions.
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

	// Check if first argument is any x.StateDir selector expression
	if len(call.Args) == 0 {
		return false
	}

	firstArg, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Match on the .StateDir field name regardless of what the receiver is named
	return firstArg.Sel.Name == "StateDir"
}

// TestHandlersUseSnapshotAccess_REQ_ARCHIMP_S18_T3 enforces the architecture:
// no non-test file in cmd/armature or internal/tui calls snapshot.Load directly.
//
// ARCHITECTURE GUARD: After the migration to snapshot.Store, all snapshot loading
// must go through Store.Load(). Direct snapshot.Load() calls
// bypass the Store and reintroduce fragmented initialization logic.
//
// This test scans cmd/armature and internal/tui sources (excluding _test.go files)
// and fails if any calls snapshot.Load( directly.
func TestHandlersUseSnapshotAccess_REQ_ARCHIMP_S18_T3(t *testing.T) {
	fset := token.NewFileSet()
	violations := []string{}

	// Find the cmd/armature directory by using runtime to locate this test file
	_, thisTestFile, _, _ := runtime.Caller(0)
	cmdDir := filepath.Dir(thisTestFile)

	// Scope: all Go files in cmd/armature, excluding _test.go
	scopeCmd, err := os.ReadDir(cmdDir)
	require.NoError(t, err)

	for _, entry := range scopeCmd {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(cmdDir, entry.Name())
		src, err := os.ReadFile(path)
		require.NoError(t, err)

		file, err := parser.ParseFile(fset, path, src, parser.AllErrors)
		require.NoError(t, err)

		// Check for direct snapshot.Load calls
		checkSnapshotLoadCalls(fset, path, file, &violations)
	}

	// Scope: all Go files in internal/tui, excluding _test.go
	// Find the root of the repo by traversing up from cmd/armature
	repoRoot := filepath.Dir(filepath.Dir(cmdDir))
	tuiAppDir := filepath.Join(repoRoot, "internal", "tui", "app")
	if info, err := os.Stat(tuiAppDir); err == nil && info.IsDir() {
		tuiFiles, err := os.ReadDir(tuiAppDir)
		require.NoError(t, err)

		for _, entry := range tuiFiles {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}

			path := filepath.Join(tuiAppDir, entry.Name())
			src, err := os.ReadFile(path)
			require.NoError(t, err)

			file, err := parser.ParseFile(fset, path, src, parser.AllErrors)
			require.NoError(t, err)

			// Check for direct snapshot.Load calls
			checkSnapshotLoadCalls(fset, path, file, &violations)
		}
	}

	if len(violations) > 0 {
		t.Fatalf("Handlers must use Store.Load() instead of direct snapshot.Load() calls:\n%s",
			strings.Join(violations, "\n"))
	}
}

// checkSnapshotLoadCalls walks the AST to find direct snapshot.Load() calls
func checkSnapshotLoadCalls(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for direct snapshot.Load calls
		if isSnapshotLoadCall(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct snapshot.Load() call detected (use Store.Load() instead)", filename, line))
		}

		return true
	})
}

// isSnapshotLoadCall detects calls to snapshot.Load(...)
func isSnapshotLoadCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Must be a method on snapshot package
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "snapshot" {
		return false
	}

	// Check for forbidden Load call
	return sel.Sel.Name == "Load"
}

// TestNoGlobalCommandRuntime_REQ_ARCHIMP_S18_T4 enforces that production commands do NOT
// read or write the process-global appCtx, appPusher, or appTracker variables.
//
// ARCHITECTURE GUARD: All execution state must flow through the Cobra command context
// via the executionStateKey, not through package-level globals. This ensures:
// - Independent commands cannot observe each other's state
// - State isolation is enforced at build time
// - Fallback behavior is eliminated
//
// This test scans all non-test .go files in cmd/armature — with no exemptions — and
// fails if any file declares appCtx, appPusher, or appTracker at package level. Any
// read of such a global requires the package-level declaration to exist to compile,
// so rejecting the declaration everywhere prevents reintroduction. (Locals named
// appCtx bound via `appCtx, err := currentCtx(cmd)` are legitimate and not flagged.)
func TestNoGlobalCommandRuntime_REQ_ARCHIMP_S18_T4(t *testing.T) {
	fset := token.NewFileSet()
	violations := []string{}

	// Find the cmd/armature directory by using runtime to locate this test file
	_, thisTestFile, _, _ := runtime.Caller(0)
	cmdDir := filepath.Dir(thisTestFile)

	// Scope: all Go files in cmd/armature, excluding _test.go
	scopeFiles, err := os.ReadDir(cmdDir)
	require.NoError(t, err)

	for _, entry := range scopeFiles {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(cmdDir, entry.Name())
		src, err := os.ReadFile(path)
		require.NoError(t, err)

		file, err := parser.ParseFile(fset, path, src, parser.AllErrors)
		require.NoError(t, err)

		// Check for references to appCtx, appPusher, appTracker
		checkGlobalCommandRuntimeUsage(fset, path, file, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("Production commands must NOT use global appCtx, appPusher, or appTracker variables.\n"+
			"Use stateFromCmd(cmd) or currentCtx(cmd) to get execution state from the command context:\n%s",
			strings.Join(violations, "\n"))
	}
}

// checkGlobalCommandRuntimeUsage walks the AST to find references to the process-global
// execution state variables. It uses a context-aware approach to avoid flagging
// local variables that shadow the globals.
func checkGlobalCommandRuntimeUsage(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	// Scan for any global variable declarations of appCtx, appPusher, or appTracker
	// which should not exist in production code anymore.
	ast.Inspect(file, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}

		// Look for var or const declarations
		if genDecl.Tok != token.VAR && genDecl.Tok != token.CONST {
			return true
		}

		// Check if any spec declares the forbidden globals
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range valueSpec.Names {
				if name.Name == "appCtx" || name.Name == "appPusher" || name.Name == "appTracker" {
					line := fset.Position(name.Pos()).Line
					msg := fmt.Sprintf("%s:%d: global variable %q must not exist; use stateFromCmd(cmd) or currentCtx(cmd) instead", filename, line, name.Name)
					*violations = append(*violations, msg)
				}
			}
		}

		return true
	})
}

// TestCommandRuntimeIsolation_REQ_ARCHIMP_S18_T4 proves that independent root commands
// cannot observe each other's execution state.
//
// This test creates two separate command trees, sets different execution state in each,
// and verifies that state from one command cannot be read by another. This demonstrates
// that execution state is properly isolated via the Cobra context, not via process globals.
func TestCommandRuntimeIsolation_REQ_ARCHIMP_S18_T4(t *testing.T) {
	// Create a mock execution state
	ctx1 := &config.Context{
		RepoPath:  "/repo1",
		IssuesDir: "/repo1/.armature",
		StateDir:  "/repo1/.armature/state",
	}

	ctx2 := &config.Context{
		RepoPath:  "/repo2",
		IssuesDir: "/repo2/.armature",
		StateDir:  "/repo2/.armature/state",
	}

	// Create first command with execution state 1
	cmd1 := &cobra.Command{
		Use: "test1",
		RunE: func(cmd *cobra.Command, args []string) error {
			state1, err := stateFromCmd(cmd)
			require.NoError(t, err)
			require.Equal(t, ctx1.RepoPath, state1.ctx.RepoPath, "cmd1 must see ctx1's state")
			return nil
		},
	}

	// Create second command with execution state 2
	cmd2 := &cobra.Command{
		Use: "test2",
		RunE: func(cmd *cobra.Command, args []string) error {
			state2, err := stateFromCmd(cmd)
			require.NoError(t, err)
			require.Equal(t, ctx2.RepoPath, state2.ctx.RepoPath, "cmd2 must see ctx2's state")
			return nil
		},
	}

	// Set different contexts on each command
	baseCtx1 := context.WithValue(context.Background(), executionStateKey{}, &executionState{ctx: ctx1})
	cmd1.SetContext(baseCtx1)

	baseCtx2 := context.WithValue(context.Background(), executionStateKey{}, &executionState{ctx: ctx2})
	cmd2.SetContext(baseCtx2)

	// Run both commands — they should each see their own isolated state
	require.NoError(t, cmd1.RunE(cmd1, nil))
	require.NoError(t, cmd2.RunE(cmd2, nil))
}
