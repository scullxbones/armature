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

// TestInteractiveTUIConstructionLivesInTuiFiles_REQ_LNGHZN_S6_T4 is the
// architecture guard for the TUI seam: interactive program construction and
// model wiring live only in cmd/armature/*_tui.go, so coverage and mutation
// gates can exclude that boundary without dropping non-interactive logic.
func TestInteractiveTUIConstructionLivesInTuiFiles_REQ_LNGHZN_S6_T4(t *testing.T) {
	t.Parallel()

	dir := cmdArmatureDir(t)
	fset := token.NewFileSet()

	for _, host := range seamHosts {
		seam := tuiSeamFile(host)
		hostPath := filepath.Join(dir, host)
		seamPath := filepath.Join(dir, seam)

		_, err := os.Stat(seamPath)
		require.NoError(t, err, "expected TUI seam file %s next to %s", seam, host)

		hostFile := parseGoFile(t, fset, hostPath)
		seamFile := parseGoFile(t, fset, seamPath)

		hostHits := teaNewProgramCalls(fset, hostFile)
		require.Empty(t, hostHits, "%s must not construct a bubbletea program; move tea.NewProgram to %s", host, seam)

		seamHits := teaNewProgramCalls(fset, seamFile)
		require.NotEmpty(t, seamHits, "%s must contain tea.NewProgram (the TUI boundary)", seam)
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

func teaNewProgramCalls(fset *token.FileSet, file *ast.File) []string {
	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewProgram" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "tea" {
			return true
		}
		hits = append(hits, fset.Position(call.Pos()).String())
		return true
	})
	return hits
}
