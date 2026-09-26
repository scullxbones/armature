package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var tuiHostConstruction = map[string][]string{
	"ready.go":       {"readytui.New", "tea.NewProgram"},
	"stalereview.go": {"stalereview.New", "tea.NewProgram"},
	"dagsum.go":      {"dagsummary.New", "tea.NewProgram"},
	"tui.go":         {"app.New", "WithScreens", "tea.NewProgram"},
}

func cmdArmatureDir(t *testing.T) string {
	t.Helper()
	_, thisTestFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(thisTestFile)
}

func cmdArmatureProductionGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(cmdArmatureDir(t))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	require.NotEmpty(t, names, "expected production Go files in cmd/armature")
	return names
}

func TestNoStandaloneTUICompanionFiles_REQ_LNGHZN_S6_T4(t *testing.T) {
	t.Parallel()

	for _, name := range cmdArmatureProductionGoFiles(t) {
		require.False(t, strings.HasSuffix(name, "_tui.go"),
			"TUI construction belongs in the command file; remove standalone %s", name)
	}
}

func TestInteractiveTUIConstructionLivesInCommandFiles_REQ_LNGHZN_S6_T4(t *testing.T) {
	t.Parallel()

	dir := cmdArmatureDir(t)
	fset := token.NewFileSet()
	names := cmdArmatureProductionGoFiles(t)

	for _, host := range []string{"ready.go", "stalereview.go", "dagsum.go", "tui.go"} {
		require.Contains(t, names, host, "expected command file %s", host)
	}

	for host, required := range tuiHostConstruction {
		hits := tuiSeamCalls(fset, parseGoFile(t, fset, filepath.Join(dir, host)))
		got := make(map[string]bool, len(hits))
		for _, hit := range hits {
			got[hit.kind] = true
		}
		for _, kind := range required {
			require.True(t, got[kind], "%s must contain %s (TUI construction / model wiring)", host, kind)
		}
	}
}

func parseGoFile(t *testing.T, fset *token.FileSet, path string) *ast.File {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	file, err := parser.ParseFile(fset, path, src, parser.AllErrors)
	require.NoError(t, err)
	return file
}

type tuiSeamHit struct {
	kind string
	pos  string
}

func tuiSeamCalls(fset *token.FileSet, file *ast.File) []tuiSeamHit {
	aliases := importAliases(file)
	var hits []tuiSeamHit
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		kind, ok := classifyTUISeamCall(sel, aliases)
		if !ok {
			return true
		}
		hits = append(hits, tuiSeamHit{kind: kind, pos: fset.Position(call.Pos()).String()})
		return true
	})
	return hits
}

func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

func classifyTUISeamCall(sel *ast.SelectorExpr, aliases map[string]string) (string, bool) {
	if sel.Sel.Name == "WithScreens" {
		return "WithScreens", true
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	pkgPath, ok := aliases[ident.Name]
	if !ok {
		return "", false
	}
	switch {
	case sel.Sel.Name == "NewProgram" && strings.HasSuffix(pkgPath, "github.com/charmbracelet/bubbletea"):
		return "tea.NewProgram", true
	case sel.Sel.Name == "New" && pkgPath == "github.com/scullxbones/armature/internal/tui/ready":
		return "readytui.New", true
	case sel.Sel.Name == "New" && pkgPath == "github.com/scullxbones/armature/internal/tui/app":
		return "app.New", true
	case sel.Sel.Name == "New" && pkgPath == "github.com/scullxbones/armature/internal/tui/dagsummary":
		return "dagsummary.New", true
	case sel.Sel.Name == "New" && pkgPath == "github.com/scullxbones/armature/internal/tui/stalereview":
		return "stalereview.New", true
	default:
		return "", false
	}
}
