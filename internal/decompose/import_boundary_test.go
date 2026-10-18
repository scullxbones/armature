package decompose

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProductionDecomposeDoesNotImportAdaptersOrSources(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	banned := map[string]string{
		`"github.com/scullxbones/armature/internal/adapters"`: "plan bytes are supplied by the composition root",
		`"github.com/scullxbones/armature/internal/sources"`:  "source content is borrowed, not loaded via Lifecycle",
	}
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
			if reason, hit := banned[spec.Path.Value]; hit {
				pos := fset.Position(spec.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d: %s (%s)", name, pos.Line, spec.Path.Value, reason))
			}
		}
	}
	require.Empty(t, violations, strings.Join(violations, "\n"))
}
