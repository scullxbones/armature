package promotion_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInvariantHelpersHaveNonTestCallers_REQ_TOPTIER_S18_T4 fails the build when
// TreeConsistencyViolations or FinishedWorkDestroyed only appear in tests —
// product must enforce both (doctor / claim / ValidateAlternatives).
func TestInvariantHelpersHaveNonTestCallers_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	want := map[string]int{
		"TreeConsistencyViolations":   0,
		"FinishedWorkDestroyed":       0,
		"WouldViolateTreeConsistency": 0,
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" ||
				base == ".armature" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := sel.Sel.Name
			if _, tracked := want[name]; !tracked {
				return true
			}
			// Count promotion.TreeConsistencyViolations / bare ident in package.
			if x, ok := sel.X.(*ast.Ident); ok {
				if x.Name == "promotion" || filepath.Base(filepath.Dir(path)) == "promotion" {
					want[name]++
				}
			}
			return true
		})
		// Also count unqualified calls inside package promotion.
		if filepath.Base(filepath.Dir(path)) == "promotion" {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				if _, tracked := want[ident.Name]; tracked {
					want[ident.Name]++
				}
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)

	for name, count := range want {
		assertNonZeroCaller(t, name, count)
	}
}

func assertNonZeroCaller(t *testing.T, name string, count int) {
	t.Helper()
	if count == 0 {
		t.Fatalf("%s has no non-test callers — product must enforce the invariant", name)
	}
}
