package contextreport

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

func TestProductionContextReportDoesNotImportAdaptersClaimOrConfig(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	banned := map[string]string{
		`"github.com/scullxbones/armature/internal/adapters"`: "fixture git is local; adapters stay at the composition root",
		`"github.com/scullxbones/armature/internal/claim"`:    "ready facts are assembled here; claim ownership stays in claim",
		`"github.com/scullxbones/armature/internal/config"`:   "token budget is the render-context default, not config.LoadConfig",
		`"github.com/scullxbones/armature/internal/doctor"`:   "budget gate is in-package; doctor does not own context-report",
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

func TestContextReportExportSurfaceHidesBudgetAndPricingPipeline(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	type exportBan struct {
		name   string
		reason string
	}
	bans := []exportBan{
		{"EstimateTokens", "byte-to-token conversion is stamped onto Collect rows"},
		{"BytesPerToken", "bytes/4 is the Collect estimation method"},
		{"DefaultTokenBudget", "render-context budget is applied inside Collect"},
		{"FixtureShowIssue", "the fixture issue id is a measurement detail"},
		{"EstimationMethod", "the method string is a Report field, not a package const"},
		{"ClassInvocation", "artifact class is an unexported Collect constant"},
		{"ClassBundle", "artifact class is an unexported Collect constant"},
		{"Artifact", "priced rows are Report.Artifacts, not a public type"},
		{"Budget", "budget rows stay behind the in-package gate"},
		{"BudgetFile", "budgets.json is loaded inside the package"},
		{"Violation", "over-budget rows are gateError details"},
		{"GateError", "budget failures are ordinary errors from the gate"},
		{"LoadBudgets", "Collect plus the in-package gate own budgets.json"},
		{"ParseBudgets", "budget JSON decoding stays inside the package"},
		{"Enforce", "the budget gate is not a second public entry point"},
		{"ExplicitTargets", "promise checks run in-package"},
		{"HasDatedTrimPlan", "trim-plan heading parse stays inside the package"},
		{"RaisedBudgets", "max_bytes raises are reviewed against budgets.json"},
		{"DynamicInvocationPaths", "priced command paths are Collect's inventory"},
		{"AOCEnvelopePaths", "AOC envelope rows are Collect's inventory"},
		{"BudgetsRelPath", "the budgets file path is an implementation constant"},
	}
	banned := make(map[string]string, len(bans))
	for _, ban := range bans {
		banned[ban.name] = ban.reason
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
