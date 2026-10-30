package doctor

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

func TestProductionDoctorDoesNotImportClaimValidateOrSources(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	banned := map[string]string{
		`"github.com/scullxbones/armature/internal/claim"`:    "stale claims come from ready facts, not the lease package",
		`"github.com/scullxbones/armature/internal/validate"`: "doctor emits health findings, not graph findings",
		`"github.com/scullxbones/armature/internal/sources"`:  "uncited issues are read from materialized facts",
		`"github.com/scullxbones/armature/cmd"`:               "internal packages must not import cmd",
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

func TestDoctorExportSurfaceHidesCheckEvaluators(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	banned := map[string]string{
		"RunChecks":                       "Run is the live check entry; indexed helpers stay in-package",
		"CheckD8ScopeViolations":          "D8 is wired through Run",
		"CheckD10ConfigHealth":            "UnhealthyConfig is the --fix D10 gate",
		"EvaluateD9UnrecognizedWorktrees": "D9 is wired through Run",
		"EvaluateD12OpsWorktreeLag":       "D12 is wired through Run",
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
