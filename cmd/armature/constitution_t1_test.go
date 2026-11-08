package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstitutionT1Exemption_REQ_MCPSTDIO_S1_T1(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../.."))

	constitution, err := os.ReadFile(filepath.Join(repoRoot, "CONSTITUTION.md"))
	require.NoError(t, err)
	text := string(constitution)

	require.Contains(t, text, "**Version:** 2", "CONSTITUTION.md must be version 2")
	require.Regexp(t, regexp.MustCompile(`(?m)^- \*\*T1\*\* — needs a long-running process\. One exemption: `+"`"+`arm mcp`+"`"), text,
		"T1 must name the arm mcp exemption")
	require.Contains(t, text, "opens no socket", "T1 exemption must ban sockets")
	require.Contains(t, text, "one `arm` command inside itself", "T1 exemption must require in-process arm commands")
	require.Contains(t, text, "closes stdin or signals", "T1 exemption must exit on client close or signal")
	require.Contains(t, text, "**I1 — Git-native.**", "I1 must stay")
	require.Contains(t, text, "**N2** — an agent framework.", "N2 must stay")

	agents, err := os.ReadFile(filepath.Join(repoRoot, "AGENTS.md"))
	require.NoError(t, err)
	agentsText := string(agents)
	require.Contains(t, agentsText, "**Version:** 2", "AGENTS.md must name constitution version 2")
	require.Contains(t, agentsText, "arm mcp", "AGENTS.md must name the arm mcp exemption")
	require.Contains(t, agentsText, "ADR 0024", "AGENTS.md must cite ADR 0024")

	adr, err := os.ReadFile(filepath.Join(repoRoot, "docs/adr/0024-arm-mcp-stdio-session-is-not-t1.md"))
	require.NoError(t, err)
	adrText := string(adr)
	require.Contains(t, adrText, "inferred", "ADR 0024 must record the inferred server/daemon reading")
	require.Contains(t, adrText, "ADR 0017", "ADR 0024 must leave ADR 0017 historical")
	require.True(t, strings.Contains(adrText, "harness-hook") && strings.Contains(adrText, "mcp"),
		"ADR 0024 must state Protocol Output commands are harness-hook and mcp")
}
