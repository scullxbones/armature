package ops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSchemaCarriesScaffoldingVersion(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()
	assert.Contains(t, schema, fmt.Sprintf("# scaffolding-version: %d\n", ScaffoldingVersion))

	got, ok := ParseScaffoldingVersion(schema)
	require.True(t, ok, "the generated SCHEMA must be parseable by the version reader")
	assert.Equal(t, ScaffoldingVersion, got)
}

func TestGenerateOpsGitignoreCarriesScaffoldingVersion(t *testing.T) {
	t.Parallel()
	gitignore := GenerateOpsGitignore()
	assert.Contains(t, gitignore, fmt.Sprintf("# scaffolding-version: %d\n", ScaffoldingVersion))
	assert.Contains(t, gitignore, adapters.OpsGitignore)

	got, ok := ParseScaffoldingVersion(gitignore)
	require.True(t, ok, "the generated ops gitignore must be parseable by the version reader")
	assert.Equal(t, ScaffoldingVersion, got)
}

func TestGenerateSchema_DocumentsTransitionTokenFields_REQ_TOPTIER_S11_T1(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()

	transitionHeaderPrefix := "#   " + OpTransition + ":"
	inTransitionBlock := false
	var block strings.Builder
	for _, line := range strings.Split(schema, "\n") {
		switch {
		case strings.HasPrefix(line, transitionHeaderPrefix):
			inTransitionBlock = true
		case strings.HasPrefix(line, "#   ") && strings.Contains(line, ":"):
			inTransitionBlock = false
		}
		if inTransitionBlock {
			block.WriteString(line)
			block.WriteByte('\n')
		}
	}
	documented := block.String()
	assert.Contains(t, documented, "input_tokens (optional)", "transition payload must document optional input_tokens")
	assert.Contains(t, documented, "output_tokens (optional)", "transition payload must document optional output_tokens")
	assert.Greater(t, ScaffoldingVersion, 2, "bump ScaffoldingVersion so bootstrap republishes ops/SCHEMA")
}

func TestParseScaffoldingVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    int
		wantOK  bool
	}{
		{name: "absent on a pre-version schema", content: "# Trellis Op Log Schema v1\n# no version here\n"},
		{name: "absent on empty content", content: ""},
		{name: "reads the recorded version", content: "# header\n# scaffolding-version: 7\n# more\n", want: 7, wantOK: true},
		{name: "absent when the value is not a number", content: "# scaffolding-version: next\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseScaffoldingVersion(tt.content)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
