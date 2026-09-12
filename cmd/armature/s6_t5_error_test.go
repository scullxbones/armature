package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentFacingRunEReturnsCommandFailure_REQ_LNGHZN_S6_T5(t *testing.T) {
	broken := setupBrokenOpsRepo(t)
	root := newRootCmd()
	registry := uniqueRegisteredCodes(t)

	var checked int
	walkAgentFacingRunE(root, func(cmd *cobra.Command) {
		path := commandArgv(cmd)
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			checked++
			if isProtocolOutputCommand(cmd) {
				stdout := new(bytes.Buffer)
				stderr := new(bytes.Buffer)
				_ = executeThenHandleRootError(t, stdout, stderr, failureArgv(path, broken)...)
				assert.NotContains(t, stdout.String(), `"code":"GENERAL-1"`)
				assert.NotContains(t, stdout.String(), "Error [GENERAL-1]")
				return
			}

			stdout, code := induceAgentFacingFailure(t, path, broken)
			assert.NotEqual(t, 0, code, "agent-facing %s must fail at the CLI port", strings.Join(path, " "))
			if protocolReportOnStdout(stdout) {
				assert.NotContains(t, stdout, `"code":"GENERAL-1"`)
				return
			}
			cf := assertAgentFailureEnvelope(t, stdout)
			assert.NotEqual(t, armerrors.CodeGeneral1, cf.Code, "agent-facing RunE must not wrap as GENERAL-1")
			_, ok := registry[cf.Code]
			assert.True(t, ok, "emitted code %q for %s must be registered", cf.Code, strings.Join(path, " "))
		})
	})
	require.Greater(t, checked, 20, "cobra tree walk must visit remaining agent-facing RunE commands")
}

func TestGeneral1WrapRemoved_REQ_LNGHZN_S6_T5(t *testing.T) {
	t.Parallel()
	cmdDir := cmdDir(t)
	for _, name := range []string{"helpers.go", "main.go", "map_port_error.go"} {
		src, err := os.ReadFile(filepath.Join(cmdDir, name))
		require.NoError(t, err)
		assert.NotContains(t, string(src), "armerrors.Unmapped", "%s must not call Unmapped", name)
		assert.NotContains(t, string(src), "func Unmapped", "%s must not define Unmapped", name)
	}
	errorsSrc, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "errors", "errors.go"))
	require.NoError(t, err)
	assert.NotContains(t, string(errorsSrc), "func Unmapped")
	assert.NotContains(t, string(errorsSrc), "Register(CodeGeneral1)")

	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "agent", false, errString("issue missing"))
	assert.Equal(t, 1, code)
	cf := assertAgentFailureEnvelope(t, stdout.String())
	assert.NotEqual(t, armerrors.CodeGeneral1, cf.Code)
	assert.Equal(t, armerrors.CodeIO, cf.Code)

	ledger := parseErrorContractLedger(t)
	var found bool
	for _, row := range ledger {
		if row.Code == "GENERAL-1" {
			found = true
			assert.True(t, row.Retired, "GENERAL-1 must stay on the ledger as retired")
		}
	}
	assert.True(t, found, "retired GENERAL-1 must remain on the ledger (no reuse)")
}

type stringError string

func errString(s string) error { return stringError(s) }

func (e stringError) Error() string { return string(e) }

func setupBrokenOpsRepo(t *testing.T) string {
	t.Helper()
	repo := setupRepoWithTask(t)
	opsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.RemoveAll(opsDir))
	require.NoError(t, os.WriteFile(opsDir, []byte("not-a-directory"), 0o600))
	return repo
}

func walkAgentFacingRunE(root *cobra.Command, visit func(*cobra.Command)) {
	var walk func(*cobra.Command, bool)
	walk = func(cmd *cobra.Command, isRoot bool) {
		hasRun := cmd.RunE != nil || cmd.Run != nil
		if hasRun && cmd.Name() != "help" && (isRoot || !cmd.HasAvailableSubCommands()) {
			visit(cmd)
		}
		for _, sub := range cmd.Commands() {
			walk(sub, false)
		}
	}
	walk(root, true)
}

func commandArgv(cmd *cobra.Command) []string {
	path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "arm"))
	if path == "" {
		return nil
	}
	return strings.Fields(path)
}

func failureArgv(path []string, repo string) []string {
	args := append([]string{}, path...)
	args = append(args, "--repo", repo, "--format", "agent", "--non-interactive")
	return args
}

func induceAgentFacingFailure(t *testing.T, path []string, repo string) (string, int) {
	t.Helper()
	attempts := [][]string{
		failureArgv(path, repo),
		append(failureArgv(path, repo), "task-01"),
		append(failureArgv(path, repo), "not-a-shell"),
		append(append([]string{}, path...), "--__lnghzn_s6_t5__", "--repo", repo, "--format", "agent", "--non-interactive"),
	}
	var lastOut string
	var lastCode int
	for _, args := range attempts {
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), args...)
		lastOut, lastCode = stdout.String(), code
		if code != 0 {
			return lastOut, lastCode
		}
	}
	return lastOut, lastCode
}

func TestOperationalInvalidArgumentFilenameIsNotUsage_REQ_LNGHZN_S6_T5(t *testing.T) {
	t.Parallel()
	importCmd := mustFindCommand(t, newRootCmd(), "import")
	pathErr := &os.PathError{Op: "open", Path: "invalid argument.csv", Err: os.ErrNotExist}
	mapped := mapAgentFacingError(importCmd, fmt.Errorf("read file: %w", pathErr))
	var cf *armerrors.CommandFailure
	require.ErrorAs(t, mapped, &cf)
	assert.Equal(t, codeImport1, cf.Code, "filename text must not flip an operational import failure to USAGE")
	assert.NotEqual(t, armerrors.CodeUSAGE, cf.Code)
	assert.Equal(t, 1, cf.ExitCode)
	assert.NotContains(t, strings.Join(cf.NextActions, "\n"), "arm --help")
	assert.False(t, isUsageError(pathErr))
	assert.False(t, isUsageError(fmt.Errorf("read file: %w", pathErr)))
}

func TestImportMissingFileNamedInvalidArgumentKeepsImport1_REQ_LNGHZN_S6_T5(t *testing.T) {
	repo := setupRepoWithTask(t)
	missing := filepath.Join(repo, "invalid argument.csv")
	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"import", missing, "--repo", repo, "--format", "agent", "--non-interactive")
	assert.Equal(t, 1, code)
	cf := assertAgentFailureEnvelope(t, stdout.String())
	assert.Equal(t, codeImport1, cf.Code)
	assert.Contains(t, cf.Cause, "invalid argument.csv")
	assert.NotEqual(t, 2, cf.ExitCode)
}

func TestCobraQuotedInvalidArgumentRemainsUsage_REQ_LNGHZN_S6_T5(t *testing.T) {
	t.Parallel()
	quoted := fmt.Errorf(`invalid argument "powershell" for "arm completion"`)
	assert.True(t, isUsageError(quoted))
	mapped := mapAgentFacingError(mustFindCommand(t, newRootCmd(), "completion"), quoted)
	var cf *armerrors.CommandFailure
	require.ErrorAs(t, mapped, &cf)
	assert.Equal(t, armerrors.CodeUSAGE, cf.Code)
	assert.Equal(t, 2, cf.ExitCode)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"import", "a.csv", "b.csv", "--format", "agent", "--non-interactive")
	assert.Equal(t, 2, code)
	extra := assertAgentFailureEnvelope(t, stdout.String())
	assert.Equal(t, armerrors.CodeUSAGE, extra.Code)
}

func mustFindCommand(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	t.Fatalf("command %q not registered", name)
	return nil
}

func protocolReportOnStdout(stdout string) bool {
	raw := strings.TrimSpace(stdout)
	if raw == "" || !json.Valid([]byte(raw)) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	if _, ok := payload["error"]; ok {
		return false
	}
	_, checks := payload["checks"]
	_, warnings := payload["warnings"]
	_, repoSetup := payload["repo_setup"]
	_, count := payload["count"]
	return checks || warnings || repoSetup || count
}
