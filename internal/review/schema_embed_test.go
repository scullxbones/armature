package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedConformanceSchemaMatchesDocs_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	docsPath := filepath.Join(repoRoot, "docs", "schemas", "conformance-assessment.schema.json")
	want, err := os.ReadFile(docsPath)
	require.NoError(t, err)
	require.Equal(t, string(want), string(review.ConformanceAssessmentSchemaJSON),
		"embedded schema must match docs/schemas/conformance-assessment.schema.json")
}
