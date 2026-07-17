package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestGrammarConformance_REQ_NXTTN_S5_T4 validates CLI grammar rules per the CLI Grammar Contract (ADR 0011).
// It walks the full Cobra command tree and asserts:
// 1. Zero hyphenated Use strings (except documented signal commands)
// 2. Single-issue commands use positional arg not --issue
// 3. Every structured-output command supports the --format enum
// 4. No command outside main.go calls tui.IsTerminal()
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

// checkHyphenatedUses returns violations for hyphenated Use strings not in the allowlist.
func checkHyphenatedUses(root *cobra.Command) []string {
	// Signal commands intentionally keeping hyphens per audit doc
	// (hyphenated without deep-module correspondence, evaluated and kept)
	signalCommands := map[string]bool{
		"context-history": true, // Diagnostic query tool; no plain alternative
		"push-ops":        true, // Specialized ops publishing verb; no plain alternative
		"scope-delete":    true, // Scope is field, not module
		"scope-rename":    true, // Scope is field, not module
		"harness-hook":    true, // Internal integration entry point
		"worker-init":     true, // Pending architectural decision; kept flat
		"render-context":  true, // Diagnostic/agent-facing tool; no plain alternative
	}

	// Allowed hyphenated subcommands under specific parent commands
	subcommandHyphens := map[string]map[string]bool{
		"sources": {
			"accept-citation": true,
			"stale-review":    true,
		},
		"validate": {
			"doc-examples": true,
		},
	}

	var violations []string
	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		// Extract command name from Use string (before any arguments)
		useFields := strings.Fields(cmd.Use)
		if len(useFields) == 0 {
			return
		}
		cmdName := useFields[0]

		// Only check if name contains hyphen
		if !strings.Contains(cmdName, "-") {
			return
		}

		// Check if it's a signal command (top-level allowed exception)
		if signalCommands[cmdName] {
			return
		}

		// Check if it's an allowed subcommand
		if parent != nil {
			parentName := parent.Name()
			if allowed, exists := subcommandHyphens[parentName]; exists && allowed[cmdName] {
				return
			}
		}

		// If we get here, it's a hyphenated name that's not in the allowlist
		violations = append(violations, cmdName)
	})

	return violations
}

// checkSingleIssuePositionalArg returns violations for single-issue commands using --issue.
// Per the audit doc's flag-convention column, these commands are sanctioned to use --issue
// because they support both positional and --issue patterns (per Flag Convention Compliance Summary):
// claim, note, decision, amend, assign, unassign, reopen, accept-citation, source-link, show, review commits.
func checkSingleIssuePositionalArg(root *cobra.Command) []string {
	// Commands that take a single issue and are sanctioned to support BOTH positional AND --issue
	// (per cli-command-audit.md § Flag Convention Compliance Summary, these commands are documented
	// as supporting both patterns; all other single-issue commands must use positional arg only)
	allowedToHaveIssueFlag := map[string]bool{
		// Single-issue commands documented to support "positional [issue-id] or --issue" pattern
		"claim":           true, // "positional [issue-id] or --issue"
		"note":            true, // "positional [issue-id]" (and supports --issue)
		"decision":        true, // "positional [issue-id] or --issue"
		"amend":           true, // "positional [issue-id] or --issue"
		"assign":          true, // "positional [issue-id] or --issue" (from audit: "Positional arg pattern compliant")
		"unassign":        true, // "positional [issue-id] or --issue"
		"reopen":          true, // "positional [issue-id] or --issue"
		"accept-citation": true, // "compliant: positional + --issue"
		"source-link":     true, // "positional [issue-id] or --issue (repeatable)"
		"show":            true, // "positional [issue-id ...] or --issue"
		"review":          true, // Group command; subcommands below:
		"prepare":         true, // review prepare: supports both positional and --issue
		"record":          true, // review record: supports both positional and --issue
		"commits":         true, // review commits: "positional [issue-id] or --issue"
		"context-history": true, // "--issue (required)" per audit (diagnostic signal)
		"heartbeat":       true, // "positional [issue-id] or --issue" (claim-related)
		"link":            true, // "positional [issue-id] or --issue" (dependency linking)
		"render-context":  true, // "positional [issue-id]" + --issue (agent context rendering)
		"transition":      true, // "positional [issue-id] or --issue" (status transitions)
	}

	// Commands that explicitly document [issue-id] or [node-id] positional arg in Use string
	// (these are confirmed single-issue commands; all others are multi-issue/no-issue)
	singleIssueFlagInUse := map[string]bool{}

	var violations []string
	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		cmdName := cmd.Name()
		use := cmd.Use

		// Commands with subcommands don't directly take issues; skip
		if len(cmd.Commands()) > 0 {
			return
		}

		// Check if this command is documented to take a single issue (has [issue-id] or [node-id] in Use)
		hasSingleIssueDoc := strings.Contains(use, "[issue-id]") || strings.Contains(use, "[node-id]")
		if hasSingleIssueDoc {
			singleIssueFlagInUse[cmdName] = true
		}

		// Check for --issue flag; if present on a single-issue command not in allowlist, record violation
		if cmd.Flags().Lookup("issue") != nil && hasSingleIssueDoc && !allowedToHaveIssueFlag[cmdName] {
			msg := cmdName + " documented as single-issue (" + use + ") but uses --issue flag (not in audit-sanctioned allowlist)"
			violations = append(violations, msg)
		}
	})

	return violations
}

// checkStructuredOutputFormat returns violations for structured-output commands without --format.
// Structured-output commands must expose the root's persistent --format flag at their command path.
func checkStructuredOutputFormat(root *cobra.Command) []string {
	// These are user-invocable command paths with structured-output contracts.
	// Paths, rather than command names, prevent an unrelated subcommand with the same
	// name (for example another "prepare") from satisfying the check by accident.
	structuredOutputCommands := [][]string{
		{"list"},
		{"log"},
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

	// Check if root has --format persistent flag (required for all structured-output commands)
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

// checkTTYDetectionPolicy returns violations for tui.IsTerminal() calls outside allowed files.
// Only main.go is permitted to call tui.IsTerminal() directly (via the shared
// autoDetectTTYPolicy helper). bootstrap.go used to hand-roll its own check but now
// calls autoDetectTTYPolicy(cmd.Root()) instead, so it no longer needs an allowlist entry.
func checkTTYDetectionPolicy() []string {
	allowedFiles := map[string]bool{
		"main.go": true,
	}

	var violations []string

	// Determine the cmd/armature directory path
	// The test runs from within cmd/armature, so try relative paths first
	cmdDir := "."
	if _, err := os.Stat(cmdDir); err != nil {
		// Fallback: try cmd/armature from working directory
		cmdDir = "cmd/armature"
		if _, err := os.Stat(cmdDir); err != nil {
			// Last resort: try absolute path if we can determine it
			wd, err := os.Getwd()
			if err == nil {
				// Remove cmd/armature from path if it's already there
				if strings.HasSuffix(wd, "cmd/armature") {
					cmdDir = wd
				} else {
					cmdDir = filepath.Join(wd, "cmd/armature")
				}
			}
		}
	}

	// Read cmd/armature directory
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return []string{"failed to read cmd/armature: " + err.Error()}
	}

	// Search each .go file (except _test.go) for tui.IsTerminal()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip test files
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		// Read file
		filePath := filepath.Join(cmdDir, name)
		content, err := os.ReadFile(filePath)
		if err != nil {
			violations = append(violations, "failed to read "+name+": "+err.Error())
			continue
		}

		// Check for tui.IsTerminal() calls
		if strings.Contains(string(content), "tui.IsTerminal(") {
			// File contains the call; check if it's allowed
			if !allowedFiles[name] {
				violations = append(violations, name+" calls tui.IsTerminal() but is not in allowlist (main.go only)")
			}
		}
	}

	return violations
}

// testNegativeCase verifies that the checker functions correctly identify violations.
// This builds synthetic command trees with intentional violations and asserts the checkers catch them.
func testNegativeCase(t *testing.T) {
	t.Run("DetectHyphenatedCommand", func(t *testing.T) {
		// Create a root with a hyphenated command not in the allowlist
		root := &cobra.Command{Use: "test-root"}
		badCmd := &cobra.Command{Use: "bad-cmd"}
		root.AddCommand(badCmd)

		violations := checkHyphenatedUses(root)
		if len(violations) == 0 {
			t.Error("checkHyphenatedUses should detect 'bad-cmd' but did not")
		}
		// Verify it detected the right violation
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
		// Create a command that uses --issue flag without being in the allowlist
		// Must have [issue-id] in Use string to be detected as single-issue command
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
		// Verify it detected the right violation
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
		// Create a root without --format flag
		root := &cobra.Command{Use: "test-root"}
		// Don't add the --format flag
		structuredCmd := &cobra.Command{Use: "list"}
		root.AddCommand(structuredCmd)

		violations := checkStructuredOutputFormat(root)
		if len(violations) == 0 {
			t.Error("checkStructuredOutputFormat should detect missing --format but did not")
		}
		// Should mention --format persistent flag
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

// Helper tests for structure and convention compliance

// TestCommandTreeStructure_REQ_NXTTN_S5_T4 validates basic command tree structure.
func TestCommandTreeStructure_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	// Verify root has expected command groups
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

	// Verify expected top-level commands exist
	expectedCommands := []string{
		"claim", "transition", "ready", "heartbeat",
		"note", "decision", "amend", "confirm", "assign", "unassign", "reopen",
		"link", "unlink",
		"sync", "push-ops", "merged", "materialize", "import",
		"version", "worker-init", "bootstrap",
		"create", "reparent", "validate", "render-context", "log",
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

// TestUseStringFormatting_REQ_NXTTN_S5_T4 validates that Use strings follow Cobra conventions.
func TestUseStringFormatting_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		use := cmd.Use
		if use == "" {
			t.Errorf("command %q has empty Use string", cmd.Name())
			return
		}

		// Use string should not start with uppercase (Cobra convention)
		if len(use) > 0 && use[0] >= 'A' && use[0] <= 'Z' {
			t.Logf("info: command %q Use string starts with uppercase (Cobra convention is lowercase)", cmd.Name())
		}

		// Use string should be reasonably short (sanity check)
		if len(use) > 100 {
			t.Logf("warning: command %q Use string is very long: %q", cmd.Name(), use)
		}

		// Check for invalid patterns
		if strings.Contains(use, "  ") {
			t.Errorf("command %q Use string has double spaces: %q", cmd.Name(), use)
		}
	})
}

// TestFormatFlagExistence_REQ_NXTTN_S5_T4 validates that format flag is properly defined.
func TestFormatFlagExistence_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	// Root should have --format flag
	if root.PersistentFlags().Lookup("format") == nil {
		t.Error("root command missing --format persistent flag")
	}

	// --format should accept values: human, json, agent
	formatFlag := root.PersistentFlags().Lookup("format")
	if formatFlag == nil {
		t.Error("--format flag not found")
		return
	}

	// Verify it's a string flag
	if formatFlag.Value.Type() != "string" {
		t.Errorf("--format flag should be string type, got %s", formatFlag.Value.Type())
	}

	// Default should be "human"
	defaultValue := formatFlag.DefValue
	if defaultValue != "human" {
		t.Errorf("--format flag default should be 'human', got %q", defaultValue)
	}
}

// TestNonInteractiveFlagExistence_REQ_NXTTN_S5_T4 validates that --non-interactive flag exists.
func TestNonInteractiveFlagExistence_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	// Root should have --non-interactive flag
	if root.PersistentFlags().Lookup("non-interactive") == nil {
		t.Error("root command missing --non-interactive persistent flag")
	}

	// --non-interactive should be a boolean
	niFlag := root.PersistentFlags().Lookup("non-interactive")
	if niFlag == nil {
		t.Error("--non-interactive flag not found")
		return
	}

	if niFlag.Value.Type() != "bool" {
		t.Errorf("--non-interactive flag should be bool type, got %s", niFlag.Value.Type())
	}

	// Default should be false
	if niFlag.DefValue != "false" {
		t.Errorf("--non-interactive flag default should be 'false', got %q", niFlag.DefValue)
	}
}

// TestPositionalArgumentPatterns_REQ_NXTTN_S5_T4 validates positional argument Use string patterns.
func TestPositionalArgumentPatterns_REQ_NXTTN_S5_T4(t *testing.T) {
	root := newRootCmd()

	// Pattern to detect positional arguments in Use string
	positionalPattern := regexp.MustCompile(`\[[a-z-]+\]`)

	walkCommandTree(root, func(cmd *cobra.Command, parent *cobra.Command) {
		use := cmd.Use
		if !positionalPattern.MatchString(use) {
			return
		}

		// Command has positional args in Use string
		// Verify it's declared correctly in Args field
		if cmd.Args == nil && len(cmd.Commands()) == 0 {
			t.Logf("info: command %q has positional arg in Use but Args is nil", cmd.Name())
		}
	})
}

// TestNoHyphenatedRootTopLevel_REQ_NXTTN_S5_T4 verifies compliance with the no-hyphens rule for commands not in the signal allowlist.
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
	}

	for _, cmd := range root.Commands() {
		cmdName := cmd.Name()
		if strings.Contains(cmdName, "-") && !signalAllowlist[cmdName] {
			t.Errorf("root-level command %q is hyphenated but not in signal allowlist", cmdName)
		}
	}
}

// walkCommandTree recursively walks the Cobra command tree, invoking visit for each command.
// The parent parameter is the parent command (nil for root).
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
