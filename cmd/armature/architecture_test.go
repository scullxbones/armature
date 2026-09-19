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

func TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6(t *testing.T) {
	scope := []string{
		"create.go", "assign.go", "dagsum.go", "confirm.go", "list.go",
		"context_history.go", "decompose.go", "hook.go",
		"scope_delete.go", "merged.go", "scope_rename.go", "ready.go",
		"reparent.go", "show.go", "stalereview.go", "sync.go", "tui.go", "validate.go",
		"transition.go",
	}

	exempt := map[string]bool{
		"render_context.go": true,
		"claim.go":          true,
		"materialize.go":    true,
	}

	fset := token.NewFileSet()
	violations := []string{}

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

		checkFile(fset, path, file, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("handlers must use newSnapshotStore(ctx) instead of direct materialize calls:\n%s",
			strings.Join(violations, "\n"))
	}
}

func checkFile(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isMaterializeCall(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct materialize call detected", filename, line))
		}

		if isStatePathJoin(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct state path construction with filepath.Join", filename, line))
		}
		return true
	})
}

func isMaterializeCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "materialize" {
		return false
	}

	methodName := sel.Sel.Name
	forbidden := map[string]bool{
		"Materialize":               true,
		"MaterializeAndReturn":      true,
		"MaterializeAndReturnQuiet": true,
		"LoadIssue":                 true,
		"LoadIndex":                 true,
	}

	if methodName == "MaterializeAtSHA" {
		return false
	}

	return forbidden[methodName]
}

func isStatePathJoin(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "filepath" || sel.Sel.Name != "Join" {
		return false
	}

	if len(call.Args) == 0 {
		return false
	}

	firstArg, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok {
		return false
	}

	return firstArg.Sel.Name == "StateDir"
}

func TestHandlersUseSnapshotAccess_REQ_ARCHIMP_S18_T3(t *testing.T) {
	fset := token.NewFileSet()
	violations := []string{}

	_, thisTestFile, _, _ := runtime.Caller(0)
	cmdDir := filepath.Dir(thisTestFile)

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

		checkSnapshotLoadCalls(fset, path, file, &violations)
	}

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

			checkSnapshotLoadCalls(fset, path, file, &violations)
		}
	}

	if len(violations) > 0 {
		t.Fatalf("Handlers must use Store.Load() instead of direct snapshot.Load() calls:\n%s",
			strings.Join(violations, "\n"))
	}
}

func checkSnapshotLoadCalls(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isSnapshotLoadCall(node) {
			line := fset.Position(node.Pos()).Line
			*violations = append(*violations, fmt.Sprintf("%s:%d: direct snapshot.Load() call detected (use Store.Load() instead)", filename, line))
		}

		return true
	})
}

func isSnapshotLoadCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "snapshot" {
		return false
	}

	return sel.Sel.Name == "Load"
}

func TestNoGlobalCommandRuntime_REQ_ARCHIMP_S18_T4(t *testing.T) {
	fset := token.NewFileSet()
	violations := []string{}

	_, thisTestFile, _, _ := runtime.Caller(0)
	cmdDir := filepath.Dir(thisTestFile)

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

		checkGlobalCommandRuntimeUsage(fset, path, file, &violations)
	}

	if len(violations) > 0 {
		t.Fatalf("Production commands must NOT use global appCtx, appPusher, or appTracker variables.\n"+
			"Use stateFromCmd(cmd) or currentCtx(cmd) to get execution state from the command context:\n%s",
			strings.Join(violations, "\n"))
	}
}

func checkGlobalCommandRuntimeUsage(fset *token.FileSet, filename string, file *ast.File, violations *[]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}

		if genDecl.Tok != token.VAR && genDecl.Tok != token.CONST {
			return true
		}

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

func TestCommandRuntimeIsolation_REQ_ARCHIMP_S18_T4(t *testing.T) {
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

	cmd1 := &cobra.Command{
		Use: "test1",
		RunE: func(cmd *cobra.Command, args []string) error {
			state1, err := stateFromCmd(cmd)
			require.NoError(t, err)
			require.Equal(t, ctx1.RepoPath, state1.ctx.RepoPath, "cmd1 must see ctx1's state")
			return nil
		},
	}

	cmd2 := &cobra.Command{
		Use: "test2",
		RunE: func(cmd *cobra.Command, args []string) error {
			state2, err := stateFromCmd(cmd)
			require.NoError(t, err)
			require.Equal(t, ctx2.RepoPath, state2.ctx.RepoPath, "cmd2 must see ctx2's state")
			return nil
		},
	}

	baseCtx1 := context.WithValue(context.Background(), executionStateKey{}, &executionState{ctx: ctx1})
	cmd1.SetContext(baseCtx1)

	baseCtx2 := context.WithValue(context.Background(), executionStateKey{}, &executionState{ctx: ctx2})
	cmd2.SetContext(baseCtx2)

	require.NoError(t, cmd1.RunE(cmd1, nil))
	require.NoError(t, cmd2.RunE(cmd2, nil))
}

func TestStatsAndShowDeriveSpendFromCapturedOps(t *testing.T) {
	_, thisTestFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisTestFile)
	for _, name := range []string{"stats.go", "show.go"} {
		src, err := os.ReadFile(filepath.Join(baseDir, name))
		require.NoError(t, err)
		if strings.Contains(string(src), "stats.LoadOps") {
			t.Errorf("%s must estimate spend from snap.Ops, not a second stats.LoadOps read", name)
		}
	}
}
