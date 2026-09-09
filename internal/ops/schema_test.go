package ops

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateSchemaCarriesScaffoldingVersion verifies that SCHEMA records the
// generator's scaffolding version, so bootstrap can tell a newer generator from
// an older one and refuse to downgrade shared scaffolding.
func TestGenerateSchemaCarriesScaffoldingVersion(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()
	assert.Contains(t, schema, fmt.Sprintf("# scaffolding-version: %d\n", ScaffoldingVersion))

	got, ok := ParseScaffoldingVersion(schema)
	require.True(t, ok, "the generated SCHEMA must be parseable by the version reader")
	assert.Equal(t, ScaffoldingVersion, got)
}

// TestParseScaffoldingVersion covers the readings bootstrap depends on: a
// pre-version SCHEMA (no header) reports absent, so it is treated as older and
// upgraded rather than parsed as version zero.
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
