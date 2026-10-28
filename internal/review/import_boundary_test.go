package review

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

func TestProductionReviewDoesNotImportOpsAdaptersConfigOrCommitref(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	banned := map[string]string{
		`"github.com/scullxbones/armature/internal/ops"`:       "review consumes borrowed gate-evidence JSON, not ops logs",
		`"github.com/scullxbones/armature/internal/adapters"`:  "review reads logs through the standard library",
		`"github.com/scullxbones/armature/internal/config"`:    "review excludes state-dir prefixes locally",
		`"github.com/scullxbones/armature/internal/commitref"`: "commit discovery lives at the composition root",
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

func TestReviewExportSurfaceHidesPrepareRecordPipeline(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	banned := map[string]string{
		"Record":                               "RecordWithDuplicateCheck is the record entry point",
		"BuildDiffIndex":                       "citation line checks run inside RecordWithDuplicateCheck",
		"DiffIndex":                            "diff indexing is a record/validate implementation detail",
		"ValidateResult":                       "ValidateAssessment is the validate entry point",
		"ValidateResultNoDiff":                 "schema checks run inside ValidateAssessment",
		"ValidateResultCoverage":               "coverage checks run inside ValidateAssessment",
		"ValidateActivityCitations":            "activity evidence checks run inside ValidateAssessment",
		"ValidateActivityDigestAndLoadEntries": "activity log loading stays inside the package",
		"FormatActivityEntryDetails":           "activity details are attached during record",
		"ActivityEntryDetails":                 "activity log rows are not a public type",
		"NewAttestation":                       "attestations are produced by RecordWithDuplicateCheck",
		"IsDuplicate":                          "duplicate detection is owned by RecordWithDuplicateCheck",
		"DeriveRating":                         "rating is stamped onto the attestation",
		"CountCriteria":                        "criterion counts are stamped onto the attestation",
		"FingerprintContract":                  "Prepare and RecordWithDuplicateCheck own fingerprints",
		"FingerprintDelivery":                  "Prepare owns delivery fingerprints",
		"FingerprintResult":                    "result fingerprints stay inside record",
		"FingerprintActivity":                  "Prepare owns activity digests",
		"ParseCriterionStatus":                 "status is decoded via ConformanceAssessment JSON",
		"BundleIDError":                        "bundle-id failures are ordinary validate/prepare errors",
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
