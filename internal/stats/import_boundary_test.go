package stats

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProductionStatsImportsStayInsideCostBoundary locks the depguard
// stats-boundary allow-list: spend estimation reads ops, attestation
// payloads, and scope overlap, and does not grow a second projection stack.
func TestProductionStatsImportsStayInsideCostBoundary(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	allowed := map[string]struct{}{
		"github.com/scullxbones/armature/internal/attestation": {},
		"github.com/scullxbones/armature/internal/ops":         {},
		"github.com/scullxbones/armature/internal/scopematch":  {},
	}

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)
	fset := token.NewFileSet()
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(pkgDir, name), nil, parser.ImportsOnly)
		require.NoError(t, parseErr, "parse %s", name)
		for _, spec := range file.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			require.NoError(t, unquoteErr)
			if !strings.Contains(path, ".") {
				continue
			}
			if _, ok := allowed[path]; ok {
				continue
			}
			pos := fset.Position(spec.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: %s", name, pos.Line, path))
		}
	}
	require.Empty(t, violations, "stats production imports outside stats-boundary:\n%s", strings.Join(violations, "\n"))
}
