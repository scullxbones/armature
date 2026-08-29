package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var failureCodeNumber = regexp.MustCompile(`^[0-9]+$`)

func TestErrorCodeRegistryUnique_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	codes := registeredFailureCodes(t)
	require.NotEmpty(t, codes, "registry must contain Failure Codes")
	seen := map[string]int{}
	for _, code := range codes {
		seen[code]++
	}
	for code, n := range seen {
		assert.Equal(t, 1, n, "Failure Code %q registered %d times; codes must be unique", code, n)
	}

	ledger := parseErrorContractLedger(t)
	ledgerSeen := map[string]int{}
	for _, row := range ledger {
		ledgerSeen[row.Code]++
	}
	for code, n := range ledgerSeen {
		assert.Equal(t, 1, n, "Failure Code %q appears %d times on the ledger; retired codes stay listed but are never reused", code, n)
	}
}

func TestErrorCodeLedgerMatchesRegistry_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	registry := uniqueRegisteredCodes(t)
	ledger := parseErrorContractLedger(t)
	require.NotEmpty(t, ledger, "docs/error-contract.md must list Failure Codes")

	liveLedger := map[string]ledgerRow{}
	retired := map[string]ledgerRow{}
	for _, row := range ledger {
		if row.Retired {
			retired[row.Code] = row
			continue
		}
		liveLedger[row.Code] = row
	}

	for code := range registry {
		_, isRetired := retired[code]
		assert.False(t, isRetired, "live registry code %q is marked retired on the ledger", code)
		_, ok := liveLedger[code]
		assert.True(t, ok, "registry code %q missing from docs/error-contract.md", code)
	}
	for code := range liveLedger {
		_, ok := registry[code]
		assert.True(t, ok, "ledger code %q is not retired but is absent from the registry", code)
	}
	for code := range retired {
		_, ok := registry[code]
		assert.False(t, ok, "retired code %q must not remain in the registry (never reuse it)", code)
	}
}

func TestFailureCodePrefixMatchesModuleOrUse_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	allowed := allowedFailureCodePrefixes(t)
	for _, code := range registeredFailureCodes(t) {
		assert.Truef(t, validFailureCodeShape(code),
			"registered code %q must be PREFIX or PREFIX-N with no zero-padded suffix", code)
		prefix := failureCodePrefix(code)
		assert.Containsf(t, allowed, prefix,
			"non-reserved prefix %q (from %q) must be ToUpper(designated deep module) or ToUpper(top-level Use)", prefix, code)
	}
	for _, row := range parseErrorContractLedger(t) {
		assert.Truef(t, validFailureCodeShape(row.Code),
			"ledger code %q must be PREFIX or PREFIX-N with no zero-padded suffix", row.Code)
		prefix := failureCodePrefix(row.Code)
		assert.Containsf(t, allowed, prefix,
			"ledger code %q prefix %q is not a reserved prefix, designated deep module, or top-level Use", row.Code, prefix)
		if _, reserved := reservedFailureCodePrefixes[prefix]; reserved {
			continue
		}
		assert.Equalf(t, armerrors.Prefix(row.ModuleOrUse), prefix,
			"ledger code %q prefix must match declared Module or Use %q", row.Code, row.ModuleOrUse)
	}
}

func TestFailureCodeShapeRejectsZeroPaddedSuffix_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	assert.False(t, validFailureCodeShape("CLAIM-001"), "padded CLAIM-001 must be rejected")
	assert.False(t, validFailureCodeShape("CLAIM-01"), "padded CLAIM-01 must be rejected")
	assert.False(t, validFailureCodeShape("CLAIM-00"), "padded CLAIM-00 must be rejected")
	assert.True(t, validFailureCodeShape("CLAIM-1"))
	assert.True(t, validFailureCodeShape("CLAIM-10"))
	assert.True(t, validFailureCodeShape("USAGE"))
	assert.True(t, validFailureCodeShape("IO"))
	assert.True(t, validFailureCodeShape("RENDER-CONTEXT-1"))
}

func TestAllowedPrefixesExcludeUndesignatedInternalPackages_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	allowed := allowedFailureCodePrefixes(t)
	assert.NotContains(t, allowed, "CLOCK",
		"internal/clock is not an ADR 0004 deep module and has no top-level Use")
	assert.Contains(t, allowed, "CLAIM", "claim is a designated deep module")
	assert.Contains(t, allowed, "READY", "ready is a top-level Use")
	assert.Contains(t, allowed, "USAGE")
	assert.Contains(t, allowed, "IO")
	assert.Contains(t, allowed, "GENERAL")
}

func TestLedgerRowPrefixMustMatchDeclaredModule_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	swapped := ledgerRow{Code: "CLAIM-1", ModuleOrUse: "review"}
	assert.NotEqual(t, failureCodePrefix(swapped.Code), armerrors.Prefix(swapped.ModuleOrUse),
		"CLAIM-1 declared as review must not satisfy the per-row prefix rule")
	matched := ledgerRow{Code: "CLAIM-1", ModuleOrUse: "claim"}
	assert.Equal(t, armerrors.Prefix(matched.ModuleOrUse), failureCodePrefix(matched.Code))
}

func TestErrorObjectRejectsExtraKeys_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	exact := map[string]any{
		"code": "CLAIM-1", "cause": "x", "next_actions": []any{}, "exit_code": 1.0,
	}
	assert.True(t, errorObjectHasExactContractKeys(exact))
	extra := map[string]any{
		"code": "CLAIM-1", "cause": "x", "next_actions": []any{}, "exit_code": 1.0, "hint": "nope",
	}
	assert.False(t, errorObjectHasExactContractKeys(extra))
	missing := map[string]any{
		"code": "CLAIM-1", "cause": "x", "next_actions": []any{},
	}
	assert.False(t, errorObjectHasExactContractKeys(missing))
}

func TestErrorObjectNotNestedInAOCEnvelope_REQ_LNGHZN_S6_T3(t *testing.T) {
	t.Parallel()
	cf := armerrors.New("CLAIM-1", "issue missing", []string{"arm ready", "arm list"}, 1)
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "agent", cf)

	raw := strings.TrimSpace(buf.String())
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON object: %q", raw)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	_, hasCount := envelope["count"]
	_, hasPayload := envelope["payload"]
	_, hasHelp := envelope["help"]
	assert.False(t, hasCount, "Command Failure must not nest in the AOC success envelope")
	assert.False(t, hasPayload, "Command Failure must not nest in the AOC success envelope")
	assert.False(t, hasHelp, "Command Failure must not nest in the AOC success envelope")
	require.Len(t, envelope, 1, "stdout object must be exactly {error:{...}}")

	errObj, ok := envelope["error"].(map[string]any)
	require.True(t, ok, "stdout object must be {error:{...}}")
	assert.True(t, errorObjectHasExactContractKeys(errObj), "error object must have exactly the four contractual keys")
	assert.Equal(t, "CLAIM-1", errObj["code"])
	assert.Equal(t, "issue missing", errObj["cause"])
	assert.Equal(t, float64(1), errObj["exit_code"])
	actions, ok := errObj["next_actions"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"arm ready", "arm list"}, actions)

	var round armerrors.CommandFailure
	inner, err := json.Marshal(errObj)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(inner, &round))
	assert.Equal(t, cf.Code, round.Code)
	assert.Equal(t, cf.Cause, round.Cause)
	assert.Equal(t, cf.NextActions, round.NextActions)
	assert.Equal(t, cf.ExitCode, round.ExitCode)
}

func TestMigratedCommandsEmitValidAgentEnvelope_REQ_LNGHZN_S6_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	registry := uniqueRegisteredCodes(t)

	readyRepo := setupRepoWithTask(t)
	opsDir := filepath.Join(readyRepo, ".armature", "ops")
	require.NoError(t, os.RemoveAll(opsDir))
	require.NoError(t, os.WriteFile(opsDir, []byte("not-a-directory"), 0o600))

	cases := []struct {
		name string
		args []string
	}{
		{"claim", []string{"claim", "--repo", repo, "--issue", "task-01", "--format", "agent"}},
		{"review", []string{"review", "prepare", "--repo", repo, "--issue", "no-such-issue", "--base", "HEAD", "--head", "HEAD", "--format", "agent"}},
		{"transition", []string{"transition", "--repo", repo, "--issue", "task-01", "--to", "nope", "--format", "agent"}},
		{"render-context", []string{"render-context", "--repo", repo, "--issue", "missing-issue", "--format", "agent"}},
		{"ready", []string{"ready", "--repo", readyRepo, "--format", "agent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), tc.args...)
			assert.NotEqual(t, 0, code)
			cf := assertAgentFailureEnvelope(t, stdout.String())
			_, registered := registry[cf.Code]
			assert.True(t, registered, "emitted code %q must be on the Failure Code registry", cf.Code)
		})
	}
}

type ledgerRow struct {
	Code         string
	ModuleOrUse  string
	Meaning      string
	FirstShipped string
	Retired      bool
}

func parseErrorContractLedger(t *testing.T) []ledgerRow {
	t.Helper()
	path := filepath.Join(repoRoot(t), "docs", "error-contract.md")
	doc, err := os.ReadFile(path)
	require.NoError(t, err, "docs/error-contract.md is the Failure Code ledger")

	var rows []ledgerRow
	inTable := false
	headerOK := false
	for _, line := range strings.Split(string(doc), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if inTable && headerOK {
				break
			}
			continue
		}
		cells := splitMarkdownRow(trimmed)
		if len(cells) < 4 {
			continue
		}
		if !headerOK {
			if strings.EqualFold(cells[0], "Code") &&
				strings.Contains(strings.ToLower(cells[1]), "module") &&
				strings.EqualFold(cells[2], "Meaning") &&
				strings.Contains(strings.ToLower(cells[3]), "first shipped") {
				headerOK = true
				inTable = true
			}
			continue
		}
		if strings.HasPrefix(cells[0], "---") || strings.HasPrefix(cells[0], ":--") {
			continue
		}
		code := strings.Trim(cells[0], "`")
		require.NotEmpty(t, code, "ledger row missing code")
		moduleOrUse := strings.Trim(cells[1], "`")
		require.NotEmpty(t, moduleOrUse, "ledger row %q missing module or Use", code)
		meaning := cells[2]
		require.NotEmpty(t, meaning, "ledger row %q missing meaning", code)
		firstShipped := strings.Trim(cells[3], "`")
		require.NotEmpty(t, firstShipped, "ledger row %q missing first shipped", code)
		retired := false
		if len(cells) >= 5 {
			retired = retiredCell(cells[4])
		}
		rows = append(rows, ledgerRow{
			Code:         code,
			ModuleOrUse:  moduleOrUse,
			Meaning:      meaning,
			FirstShipped: firstShipped,
			Retired:      retired,
		})
	}
	require.True(t, headerOK, "docs/error-contract.md must have a table with Code, Module or Use, Meaning, First shipped")
	return rows
}

func splitMarkdownRow(line string) []string {
	raw := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(raw))
	for _, cell := range raw {
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells
}

func retiredCell(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "retired", "unused":
		return true
	default:
		return false
	}
}

func uniqueRegisteredCodes(t *testing.T) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	for _, code := range registeredFailureCodes(t) {
		out[code] = struct{}{}
	}
	return out
}

func registeredFailureCodes(t *testing.T) []string {
	t.Helper()
	var codes []string
	dirs := []string{
		cmdDir(t),
		filepath.Join(repoRoot(t), "internal", "errors"),
	}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, 0)
			require.NoError(t, err, "parse %s", path)
			consts := stringConsts(file)
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isFailureCodeRegister(call.Fun) || len(call.Args) != 1 {
					return true
				}
				code, ok := resolveStringArg(call.Args[0], consts)
				if !ok {
					t.Errorf("%s: Register() argument is not a string constant", fset.Position(call.Pos()))
					return true
				}
				codes = append(codes, code)
				return true
			})
		}
	}
	return codes
}

func isFailureCodeRegister(fun ast.Expr) bool {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name == "Register"
	case *ast.SelectorExpr:
		pkg, ok := v.X.(*ast.Ident)
		return ok && pkg.Name == "armerrors" && v.Sel.Name == "Register"
	default:
		return false
	}
}

func stringConsts(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						out[name.Name] = s
					}
				}
			}
		}
	}
	return out
}

func resolveStringArg(expr ast.Expr, consts map[string]string) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := consts[v.Name]
		return s, ok
	case *ast.SelectorExpr:
		s, ok := consts[v.Sel.Name]
		return s, ok
	default:
		return "", false
	}
}

var reservedFailureCodePrefixes = map[string]struct{}{
	"USAGE":   {},
	"IO":      {},
	"GENERAL": {},
}

// ADR 0004 designated deep modules. Not every internal/ directory.
var designatedDeepModules = []string{
	"ops", "claim", "traceability", "materialize", "sources", "validate", "output",
}

func validFailureCodeShape(code string) bool {
	if code == "" {
		return false
	}
	i := strings.LastIndex(code, "-")
	if i < 0 {
		return true
	}
	suffix := code[i+1:]
	if !failureCodeNumber.MatchString(suffix) {
		return true
	}
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return false
	}
	return strconv.Itoa(n) == suffix
}

func errorObjectHasExactContractKeys(errObj map[string]any) bool {
	if len(errObj) != 4 {
		return false
	}
	for _, field := range []string{"code", "cause", "next_actions", "exit_code"} {
		if _, present := errObj[field]; !present {
			return false
		}
	}
	return true
}

func allowedFailureCodePrefixes(t *testing.T) map[string]struct{} {
	t.Helper()
	allowed := make(map[string]struct{}, len(reservedFailureCodePrefixes)+len(designatedDeepModules))
	for prefix := range reservedFailureCodePrefixes {
		allowed[prefix] = struct{}{}
	}
	for _, name := range designatedDeepModules {
		allowed[armerrors.Prefix(name)] = struct{}{}
	}
	root := newRootCmd()
	for _, cmd := range root.Commands() {
		fields := strings.Fields(cmd.Use)
		if len(fields) == 0 {
			continue
		}
		allowed[armerrors.Prefix(fields[0])] = struct{}{}
	}
	return allowed
}

func failureCodePrefix(code string) string {
	i := strings.LastIndex(code, "-")
	if i < 0 {
		return code
	}
	if failureCodeNumber.MatchString(code[i+1:]) {
		return code[:i]
	}
	return code
}

func assertAgentFailureEnvelope(t *testing.T, stdout string) *armerrors.CommandFailure {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON object, got %q", stdout)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	_, hasCount := envelope["count"]
	_, hasPayload := envelope["payload"]
	_, hasHelp := envelope["help"]
	assert.False(t, hasCount, "Command Failure must not nest in the AOC success envelope")
	assert.False(t, hasPayload, "Command Failure must not nest in the AOC success envelope")
	assert.False(t, hasHelp, "Command Failure must not nest in the AOC success envelope")
	require.Len(t, envelope, 1)
	errObj, ok := envelope["error"].(map[string]any)
	require.True(t, ok, "stdout must be {error:{...}}")
	assert.True(t, errorObjectHasExactContractKeys(errObj), "error object must have exactly the four contractual keys")

	var typed commandFailureEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &typed))
	require.NotNil(t, typed.Error)
	return typed.Error
}

func cmdDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(cmdDir(t), "..", "..")
}
