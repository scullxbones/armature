package main

import (
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/require"
)

func TestEveryAgentFacingCommandHasConformingFixture_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	require.NoError(t, output.Lint(newRootCmd(), output.DefaultGoldenDir()),
		"every cobra-enumerated agent-facing mode needs a conforming golden under internal/output/testdata/golden")
}
