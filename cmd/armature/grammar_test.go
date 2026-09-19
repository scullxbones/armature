package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGrammarConformance_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	t.Run("HyphenatedUses", func(t *testing.T) {
		violations := checkHyphenatedUses(root)
		if len(violations) > 0 {
			t.Errorf("hyphenated Use strings found (not in allowlist):\n  %s", strings.Join(violations, "\n  "))
		}
	})

	t.Run("SingleIssuePositionalArg", func(t *testing.T) {
		violations := checkSingleIssuePositionalArg(root)
		if len(violations) > 0 {
			t.Errorf("single-issue commands using --issue flag (not in allowlist):\n  %s", strings.Join(violations, "\n  "))
		}
	})

	t.Run("StructuredOutputFormat", func(t *testing.T) {
		violations := checkStructuredOutputFormat(root)
		if len(violations) > 0 {
			t.Errorf("structured-output commands missing --format support:\n  %s", strings.Join(violations, "\n  "))
		}
	})

	t.Run("TTYDetectionPolicy", func(t *testing.T) {
		violations := checkTTYDetectionPolicy()
		if len(violations) > 0 {
			t.Errorf("tui.IsTerminal() calls outside allowed files:\n  %s", strings.Join(violations, "\n  "))
		}
	})

	t.Run("NegativeCase", func(t *testing.T) {
		testNegativeCase(t)
	})
}

func checkHyphenatedUses(root *cobra.Command) []string {
	signalCommands := map[string]bool{
		"context-history": true,
		"push-ops":        true,
		"scope-delete":    true,
		"scope-rename":    true,
		"harness-hook":    true,
		"worker-init":     true,
		"render-context":  true,
		"context-report":  true,
	}

	subcommandHyphens := map[string]map[string]bool{
		"sources": {
			"accept-citation": true,
			"stale-review":    true,
		},
		"validate": {
			"doc-examples": true,
		},
		"dag": {
			"override-release": true,
		},
	}

	var violations []string
	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		useFields := strings.Fields(cmd.Use)
		if len(useFields) == 0 {
			return
		}
		cmdName := useFields[0]

		if !strings.Contains(cmdName, "-") {
			return
		}

		if signalCommands[cmdName] {
			return
		}

		if parent != nil {
			parentName := parent.Name()
			if allowed, exists := subcommandHyphens[parentName]; exists && allowed[cmdName] {
				return
			}
		}

		violations = append(violations, cmdName)
	})

	return violations
}

func checkSingleIssuePositionalArg(root *cobra.Command) []string {
	allowedToHaveIssueFlag := map[string]bool{
		"claim":           true,
		"note":            true,
		"decision":        true,
		"amend":           true,
		"assign":          true,
		"unassign":        true,
		"reopen":          true,
		"accept-citation": true,
		"source-link":     true,
		"show":            true,
		"review":          true,
		"prepare":         true,
		"record":          true,
		"commits":         true,
		"context-history": true,
		"heartbeat":       true,
		"link":            true,
		"render-context":  true,
		"transition":      true,
	}

	singleIssueFlagInUse := map[string]bool{}

	var violations []string
	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		cmdName := cmd.Name()
		use := cmd.Use

		if len(cmd.Commands()) > 0 {
			return
		}

		hasSingleIssueDoc := strings.Contains(use, "[issue-id]") || strings.Contains(use, "[node-id]")
		if hasSingleIssueDoc {
			singleIssueFlagInUse[cmdName] = true
		}

		if cmd.Flags().Lookup("issue") != nil && hasSingleIssueDoc && !allowedToHaveIssueFlag[cmdName] {
			msg := cmdName + " documented as single-issue (" + use + ") but uses --issue flag (not in audit-sanctioned allowlist)"
			violations = append(violations, msg)
		}
	})

	return violations
}

func checkStructuredOutputFormat(root *cobra.Command) []string {
	structuredOutputCommands := [][]string{
		{"list"},
		{"log"},
		{"stats"},
		{"show"},
		{"ready"},
		{"validate"},
		{"render-context"},
		{"workers"},
		{"review", "prepare"},
		{"review", "record"},
		{"review", "commits"},
	}

	var violations []string

	rootFormatFlag := root.PersistentFlags().Lookup("format")
	if rootFormatFlag == nil {
		violations = append(violations, "root command missing --format persistent flag (required for all structured-output commands)")
	}

	for _, path := range structuredOutputCommands {
		cmd, _, err := root.Find(path)
		pathName := strings.Join(path, " ")
		if err != nil || cmd == root {
			violations = append(violations, "structured-output command not registered: "+pathName)
			continue
		}
		if cmd.Flags().Lookup("format") == nil && cmd.InheritedFlags().Lookup("format") == nil {
			violations = append(violations, "structured-output command missing --format support: "+pathName)
		}
	}

	return violations
}

func checkTTYDetectionPolicy() []string {
	allowedFiles := map[string]bool{
		"main.go": true,
	}

	var violations []string

	cmdDir := "."
	if _, err := os.Stat(cmdDir); err != nil {
		cmdDir = "cmd/armature"
		if _, err := os.Stat(cmdDir); err != nil {
			wd, err := os.Getwd()
			if err == nil {
				if strings.HasSuffix(wd, "cmd/armature") {
					cmdDir = wd
				} else {
					cmdDir = filepath.Join(wd, "cmd/armature")
				}
			}
		}
	}

	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return []string{"failed to read cmd/armature: " + err.Error()}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		filePath := filepath.Join(cmdDir, name)
		content, err := os.ReadFile(filePath)
		if err != nil {
			violations = append(violations, "failed to read "+name+": "+err.Error())
			continue
		}

		if strings.Contains(string(content), "tui.IsTerminal(") {
			if !allowedFiles[name] {
				violations = append(violations, name+" calls tui.IsTerminal() but is not in allowlist (main.go only)")
			}
		}
	}

	return violations
}

func testNegativeCase(t *testing.T) {
	t.Run("DetectHyphenatedCommand", func(t *testing.T) {
		root := &cobra.Command{Use: "test-root"}
		badCmd := &cobra.Command{Use: "bad-cmd"}
		root.AddCommand(badCmd)

		violations := checkHyphenatedUses(root)
		if len(violations) == 0 {
			t.Error("checkHyphenatedUses should detect 'bad-cmd' but did not")
		}
		found := false
		for _, v := range violations {
			if strings.Contains(v, "bad-cmd") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("checkHyphenatedUses found violations but not 'bad-cmd': %v", violations)
		}
	})

	t.Run("DetectIssueFlag", func(t *testing.T) {
		root := &cobra.Command{Use: "test-root"}
		badCmd := &cobra.Command{
			Use:   "violate [issue-id]",
			Short: "A command that violates the single-issue positional arg rule",
		}
		badCmd.Flags().String("issue", "", "issue ID (should not be here for single-issue command)")
		root.AddCommand(badCmd)

		violations := checkSingleIssuePositionalArg(root)
		if len(violations) == 0 {
			t.Error("checkSingleIssuePositionalArg should detect 'violate' but did not")
		}
		found := false
		for _, v := range violations {
			if strings.Contains(v, "violate") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("checkSingleIssuePositionalArg found violations but not 'violate': %v", violations)
		}
	})

	t.Run("DetectMissingFormat", func(t *testing.T) {
		root := &cobra.Command{Use: "test-root"}
		structuredCmd := &cobra.Command{Use: "list"}
		root.AddCommand(structuredCmd)

		violations := checkStructuredOutputFormat(root)
		if len(violations) == 0 {
			t.Error("checkStructuredOutputFormat should detect missing --format but did not")
		}
		found := false
		for _, v := range violations {
			if strings.Contains(v, "--format") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("checkStructuredOutputFormat violations should mention --format: %v", violations)
		}
	})

	t.Run("DetectMissingStructuredCommand", func(t *testing.T) {
		root := &cobra.Command{Use: "test-root"}
		root.PersistentFlags().String("format", "human", "output format")

		violations := checkStructuredOutputFormat(root)
		if len(violations) == 0 {
			t.Error("checkStructuredOutputFormat should detect missing structured commands")
		}
		found := false
		for _, violation := range violations {
			if strings.Contains(violation, "list") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("checkStructuredOutputFormat should report the missing list command: %v", violations)
		}
	})
}

func TestCommandTreeStructure_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	groups := root.Groups()
	if len(groups) == 0 {
		t.Error("root command has no groups defined")
	}

	groupNames := make(map[string]bool)
	for _, g := range groups {
		groupNames[g.ID] = true
	}

	expectedGroups := map[string]bool{
		"workflow": true,
		"dag":      true,
		"sync":     true,
		"admin":    true,
	}

	for groupID := range expectedGroups {
		if !groupNames[groupID] {
			t.Errorf("expected command group %q not found", groupID)
		}
	}

	expectedCommands := []string{
		"claim", "transition", "ready", "heartbeat",
		"note", "decision", "amend", "confirm", "assign", "unassign", "reopen",
		"link", "unlink",
		"sync", "push-ops", "merged", "materialize", "import",
		"version", "worker-init", "bootstrap",
		"create", "reparent", "validate", "render-context", "log", "stats",
		"workers", "sources", "show", "list", "scope-rename", "scope-delete",
		"doctor", "completion", "hook", "tui", "context-history", "harness-hook",
		"review",
	}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %q not found at root level", cmdName)
		}
	}
}

func TestUseStringFormatting_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		use := cmd.Use
		if use == "" {
			t.Errorf("command %q has empty Use string", cmd.Name())
			return
		}

		if len(use) > 0 && use[0] >= 'A' && use[0] <= 'Z' {
			t.Logf("info: command %q Use string starts with uppercase (Cobra convention is lowercase)", cmd.Name())
		}

		if len(use) > 100 {
			t.Logf("warning: command %q Use string is very long: %q", cmd.Name(), use)
		}

		if strings.Contains(use, "  ") {
			t.Errorf("command %q Use string has double spaces: %q", cmd.Name(), use)
		}
	})
}

func TestFormatFlagExistence_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	if root.PersistentFlags().Lookup("format") == nil {
		t.Error("root command missing --format persistent flag")
	}

	formatFlag := root.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Error("--format flag not found")
		return
	}

	if formatFlag.Value.Type() != "string" {
		t.Errorf("--format flag should be string type, got %s", formatFlag.Value.Type())
	}

	defaultValue := formatFlag.DefValue
	if defaultValue != "human" {
		t.Errorf("--format flag default should be 'human', got %q", defaultValue)
	}
}

func TestNonInteractiveFlagExistence_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	if root.PersistentFlags().Lookup("non-interactive") == nil {
		t.Error("root command missing --non-interactive persistent flag")
	}

	niFlag := root.PersistentFlags().Lookup("non-interactive")
	if niFlag == nil {
		t.Error("--non-interactive flag not found")
		return
	}

	if niFlag.Value.Type() != "bool" {
		t.Errorf("--non-interactive flag should be bool type, got %s", niFlag.Value.Type())
	}

	if niFlag.DefValue != "false" {
		t.Errorf("--non-interactive flag default should be 'false', got %q", niFlag.DefValue)
	}
}

func TestPositionalArgumentPatterns_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	positionalPattern := regexp.MustCompile(`\[[a-z-]+\]`)

	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		use := cmd.Use
		if !positionalPattern.MatchString(use) {
			return
		}

		if cmd.Args == nil && len(cmd.Commands()) == 0 {
			t.Logf("info: command %q has positional arg in Use but Args is nil", cmd.Name())
		}
	})
}

func TestNoHyphenatedRootTopLevel_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	signalAllowlist := map[string]bool{
		"context-history": true,
		"push-ops":        true,
		"scope-delete":    true,
		"scope-rename":    true,
		"harness-hook":    true,
		"worker-init":     true,
		"render-context":  true,
		"context-report":  true,
	}

	for _, cmd := range root.Commands() {
		cmdName := cmd.Name()
		if strings.Contains(cmdName, "-") && !signalAllowlist[cmdName] {
			t.Errorf("root-level command %q is hyphenated but not in signal allowlist", cmdName)
		}
	}
}

func walkCommandTree(root *cobra.Command, visit func(*cobra.Command, *cobra.Command)) {
	var walk func(*cobra.Command, *cobra.Command)
	walk = func(cmd *cobra.Command, parent *cobra.Command) {
		visit(cmd, parent)
		for _, subcmd := range cmd.Commands() {
			walk(subcmd, cmd)
		}
	}
	walk(root, nil)
}
