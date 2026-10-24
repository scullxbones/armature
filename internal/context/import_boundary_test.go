package context

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

	"github.com/stretchr/testify/require"
)

func TestProductionContextDoesNotImportMaterializeOrConfig(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	banned := map[string]string{
		`"github.com/scullxbones/armature/internal/materialize"`: "context consumes IssueFacts, not materialized issues",
		`"github.com/scullxbones/armature/internal/config"`:      "state directory name is local to context IssueFacts",
		`"github.com/scullxbones/armature/internal/ops"`:         "status vocabulary is local to context IssueFacts",
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

func TestWorkingMemoryExportSurfaceHidesAssemblyInternals(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	banned := map[string]string{
		"FileReader":    "file readers stay unexported behind Options",
		"OSFileReader":  "OS adapter stays unexported behind Options.Root",
		"Assemble":      "callers render working memory; they do not assemble layers",
		"Truncate":      "truncation is owned by RenderAgent/RenderHuman",
		"InferRepoRoot": "root inference is owned by Options.StateDir",
		"Context":       "working-memory JSON is a render result, not a caller type",
		"Layer":         "layers stay behind RenderAgent/RenderHuman",
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
		file, parseErr := parser.ParseFile(fset, filepath.Join(pkgDir, name), nil, 0)
		require.NoError(t, parseErr, "parse %s", name)
		for _, spec := range file.Decls {
			gen, ok := spec.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, s := range gen.Specs {
				switch ts := s.(type) {
				case *ast.TypeSpec:
					if reason, hit := banned[ts.Name.Name]; hit && ts.Name.IsExported() {
						violations = append(violations, fmt.Sprintf("%s: %s (%s)", name, ts.Name.Name, reason))
					}
				case *ast.ValueSpec:
					for _, ident := range ts.Names {
						if reason, hit := banned[ident.Name]; hit && ident.IsExported() {
							violations = append(violations, fmt.Sprintf("%s: %s (%s)", name, ident.Name, reason))
						}
					}
				}
			}
		}
		for _, spec := range file.Decls {
			fn, ok := spec.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}
			if reason, hit := banned[fn.Name.Name]; hit && fn.Name.IsExported() {
				violations = append(violations, fmt.Sprintf("%s: %s (%s)", name, fn.Name.Name, reason))
			}
		}
	}
	require.Empty(t, violations, strings.Join(violations, "\n"))
}
