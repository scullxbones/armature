package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMandatoryFlagsMatchMarkFlagRequired is a drift detector: it fails if a
// cmd/armature/*.go command calls cobra's MarkFlagRequired for a flag that
// isn't reflected in scripts/skill_lint.py's MANDATORY_FLAGS table. Without
// this, skill docs can show a command missing a flag the real CLI requires
// and skill-lint will happily pass it (see PR #73 review thread
// PRRT_kwDORnVQE86QmOzV).
//
// This is intentionally a coarse, file-level check rather than a full model
// of Cobra's command tree: for each cmd/armature/*.go file, it collects (a)
// every `Use: "..."` token's first word in that file, and (b) every flag
// name passed to MarkFlagRequired in that file. It then requires that, for
// each such flag, at least one MANDATORY_FLAGS key whose own first word
// appears in that file's Use set lists the flag. That's precise enough to
// catch a newly added MarkFlagRequired call that skill_lint.py doesn't know
// about, without needing to parse Cobra's nested command wiring.
func TestMandatoryFlagsMatchMarkFlagRequired(t *testing.T) {
	root := projectRootDir(t)

	mandatoryFlags := parseMandatoryFlags(t, filepath.Join(root, "scripts", "skill_lint.py"))

	cmdDir := filepath.Join(root, "cmd", "armature")
	entries, err := os.ReadDir(cmdDir)
	require.NoError(t, err)

	useRe := regexp.MustCompile(`Use:\s*"([^"]+)"`)
	requiredRe := regexp.MustCompile(`MarkFlagRequired\("([^"]+)"\)`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(cmdDir, entry.Name())
		content, err := os.ReadFile(path)
		require.NoError(t, err)

		requiredMatches := requiredRe.FindAllStringSubmatch(string(content), -1)
		if len(requiredMatches) == 0 {
			continue
		}

		useWords := map[string]bool{}
		for _, m := range useRe.FindAllStringSubmatch(string(content), -1) {
			firstWord := strings.Fields(m[1])
			if len(firstWord) == 0 {
				continue
			}
			useWords[firstWord[0]] = true
		}

		for _, m := range requiredMatches {
			flag := "--" + m[1]
			found := false
			for key, flags := range mandatoryFlags {
				keyFirstWord := strings.Fields(key)[0]
				if !useWords[keyFirstWord] {
					continue
				}
				if slices.Contains(flags, flag) {
					found = true
					break
				}
			}
			require.True(t, found,
				"cmd/armature/%s calls MarkFlagRequired(%q), but no MANDATORY_FLAGS entry in\n"+
					"scripts/skill_lint.py (key matching a Use command in this file: %v) lists %q\n"+
					"— update MANDATORY_FLAGS",
				entry.Name(), m[1], useWords, flag)
		}
	}
}

// parseMandatoryFlags extracts the MANDATORY_FLAGS dict literal from
// scripts/skill_lint.py as a map of command name -> flag list, without
// invoking Python.
func parseMandatoryFlags(t *testing.T, skillLintPath string) map[string][]string {
	t.Helper()
	content, err := os.ReadFile(skillLintPath)
	require.NoError(t, err)

	dictRe := regexp.MustCompile(`(?s)MANDATORY_FLAGS = \{(.*?)\n\}`)
	m := dictRe.FindStringSubmatch(string(content))
	require.NotNil(t, m, "could not locate MANDATORY_FLAGS dict in %s", skillLintPath)

	entryRe := regexp.MustCompile(`"([^"]+)":\s*\[([^\]]*)\]`)
	entries := entryRe.FindAllStringSubmatch(m[1], -1)
	require.NotEmpty(t, entries, "MANDATORY_FLAGS dict appears empty in %s", skillLintPath)

	result := make(map[string][]string, len(entries))
	flagRe := regexp.MustCompile(`"([^"]+)"`)
	for _, e := range entries {
		key := e[1]
		var flags []string
		for _, fm := range flagRe.FindAllStringSubmatch(e[2], -1) {
			flags = append(flags, fm[1])
		}
		result[key] = flags
	}
	return result
}

// projectRootDir locates the repository root by walking up from the current
// working directory until it finds the Makefile.
func projectRootDir(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	require.NoError(t, err)
	for !fileExists(filepath.Join(root, "Makefile")) {
		parent := filepath.Dir(root)
		require.NotEqual(t, root, parent, "repository root not found")
		root = parent
	}
	return root
}
