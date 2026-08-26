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

// seamHosts are the command files that previously mixed interactive TUI
// construction with non-interactive logic. After LNGHZN-S6-T4, tea.NewProgram
// and model wiring live only in the matching *_tui.go sibling.
var seamHosts = []string{
	"ready.go",
	"stalereview.go",
	"dagsum.go",
	"tui.go",
}

// requiredSeamWiring is the model-wiring + program-construction each seam
// file must contain. Kinds match classifyTUISeamCall.
var requiredSeamWiring = map[string][]string{
	"ready_tui.go":       {"readytui.New", "tea.NewProgram"},
	"stalereview_tui.go": {"stalereview.New", "tea.NewProgram"},
	"dagsum_tui.go":      {"dagsummary.New", "tea.NewProgram"},
	"tui_tui.go":         {"app.New", "WithScreens", "tea.NewProgram"},
}

func cmdArmatureDir(t *testing.T) string {
	t.Helper()
	_, thisTestFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(thisTestFile)
}

func repoRootFromCmd(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(cmdArmatureDir(t)))
}

func tuiSeamFile(host string) string {
	return strings.TrimSuffix(host, ".go") + "_tui.go"
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

// TestInteractiveTUIConstructionLivesInTuiFiles_REQ_LNGHZN_S6_T4 is the
// architecture guard for the TUI seam: interactive program construction and
// model wiring live only in cmd/armature/*_tui.go, so coverage and mutation
// gates can exclude that boundary without dropping non-interactive logic.
func TestInteractiveTUIConstructionLivesInTuiFiles_REQ_LNGHZN_S6_T4(t *testing.T) {
	t.Parallel()

	dir := cmdArmatureDir(t)
	fset := token.NewFileSet()

	for _, name := range cmdArmatureProductionGoFiles(t) {
		file := parseGoFile(t, fset, filepath.Join(dir, name))
		hits := tuiSeamCalls(fset, file)
		if strings.HasSuffix(name, "_tui.go") {
			continue
		}
		require.Empty(t, hits, "%s must not construct a bubbletea program or wire TUI models; move to *_tui.go: %v", name, hits)
	}

	for _, host := range seamHosts {
		seam := tuiSeamFile(host)
		seamPath := filepath.Join(dir, seam)
		_, err := os.Stat(seamPath)
		require.NoError(t, err, "expected TUI seam file %s next to %s", seam, host)

		required, ok := requiredSeamWiring[seam]
		require.True(t, ok, "requiredSeamWiring must list %s", seam)

		hits := tuiSeamCalls(fset, parseGoFile(t, fset, seamPath))
		got := make(map[string]bool, len(hits))
		for _, hit := range hits {
			got[hit.kind] = true
		}
		for _, kind := range required {
			require.True(t, got[kind], "%s must contain %s (TUI construction / model wiring)", seam, kind)
		}
	}
}

// TestGremlinsExcludesTUISeamFiles_REQ_LNGHZN_S6_T4 locks the mutation-gate
// exclusion: .gremlins.yaml exclude-files must list the *_tui.go pattern,
// same precedent as _windows.go.
func TestGremlinsExcludesTUISeamFiles_REQ_LNGHZN_S6_T4(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join(repoRootFromCmd(t), ".gremlins.yaml"))
	require.NoError(t, err)

	// The YAML stores the regexp as a quoted string, matching the
	// `_windows.go` precedent on the next line of exclude-files.
	require.Contains(t, string(raw), `"_tui\\.go$"`,
		".gremlins.yaml unleash.exclude-files must include the *_tui.go pattern")
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
