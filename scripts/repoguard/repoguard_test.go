package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestSourceManifest_omitemptyRawMessageBypass_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/validate/hole.go": `package validate
import "encoding/json"
type hole struct {
	Entries    map[string]json.RawMessage ` + "`json:\"entries,omitempty\"`" + `
	LastSynced json.RawMessage            ` + "`json:\"last_synced,omitempty\"`" + `
}
`,
	})
	src, err := os.ReadFile(filepath.Join(root, "internal/validate/hole.go"))
	if err != nil {
		t.Fatal(err)
	}
	if legacy365ExactJSONTag(string(src), "entries") || legacy365ExactJSONTag(string(src), "last_synced") {
		t.Fatal("old rg json:\"entries\" / json:\"last_synced\" must miss omitempty tags")
	}
	out, errOut, code := runCheck(t, "source-manifest", root)
	if code == 0 {
		t.Fatalf("expected fail, stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "KnownSources") || !strings.Contains(errOut, "sources.Manifest.Unmarshal") {
		t.Fatalf("fix must name Manifest.Unmarshal and KnownSources, got %s", errOut)
	}
}

func TestSourceManifest_sourcesOnlyPasses(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/sources/types.go": `package sources
type SourceEntry struct {
	LastSynced string ` + "`json:\"last_synced\"`" + `
}
type Manifest struct {
	Entries map[string]SourceEntry ` + "`json:\"entries\"`" + `
}
`,
	})
	out, errOut, code := runCheck(t, "source-manifest", root)
	if code != 0 {
		t.Fatalf("expected pass, stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "OK: source-manifest") {
		t.Fatalf("missing OK line: %s", out)
	}
}

func TestSourceManifest_notIgnoreBuildTag_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	src := `//go:build !ignore
package validate
type sourceEntry struct{}
`
	if !legacyIgnoreSubstring(src) {
		t.Fatal("old substring ignore matcher must skip //go:build !ignore")
	}
	root := writeTree(t, map[string]string{
		"go.mod":                    "module fixture\n\ngo 1.22\n",
		"internal/validate/skip.go": src,
	})
	_, errOut, code := runCheck(t, "source-manifest", root)
	if code == 0 {
		t.Fatalf("expected fail, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "sources.Manifest.Unmarshal") {
		t.Fatalf("got %s", errOut)
	}
}

func TestSourceManifest_ignoreBuildTagStillSkipped(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/validate/gen.go": `//go:build ignore
package validate
type sourceEntry struct{}
`,
	})
	out, errOut, code := runCheck(t, "source-manifest", root)
	if code != 0 {
		t.Fatalf("expected pass, stdout=%s stderr=%s", out, errOut)
	}
}

func TestSourceManifest_copiedDecoderNames(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/validate/validate.go": `package validate
type sourceEntry struct{}
func parseManifestData(data []byte) {}
`,
	})
	_, errOut, code := runCheck(t, "source-manifest", root)
	if code == 0 {
		t.Fatal("expected fail")
	}
	if !strings.Contains(errOut, "sources.Manifest.Unmarshal") {
		t.Fatalf("got %s", errOut)
	}
}

func TestTestExecutables_namedFileModeVarBypass_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := `package fake
func f() {
	mode := os.FileMode(0o755)
	_ = os.WriteFile(p, b, mode)
}
`
	if legacy368WriteFileMode.MatchString(body) {
		t.Fatal("#368 last-digit rg must miss a named FileMode var")
	}
	assertExecRejected(t, "named_mode", body)
}

func TestTestExecutables_historicalModes_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, mode string
	}{
		{"0o700", "0o700"},
		{"0o744", "0o744"},
		{"0o750", "0o750"},
		{"0o755", "0o755"},
		{"0755", "0755"},
		{"modeperm", "os.ModePerm"},
		{"fsfilemode", "fs.FileMode(0o700)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := "package fake\nfunc f() { _ = os.WriteFile(path, body, " + tc.mode + ") }\n"
			if tc.name == "0o700" || tc.name == "0o744" || tc.name == "0o750" {
				if legacy368WriteFileMode.MatchString(body) {
					t.Fatalf("#368 rg must miss %s", tc.mode)
				}
			}
			assertExecRejected(t, tc.name, body)
		})
	}
}

func TestTestExecutables_paramModeOrExec_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := `package fake
func f(perm os.FileMode) {
	_ = os.WriteFile(p, nil, perm|0o100)
}
`
	assertExecRejected(t, "param_or", body)
}

func TestTestExecutables_andNotMode_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := `package fake
func f() {
	_ = os.WriteFile(p, nil, 0o755&^0o022)
}
`
	assertExecRejected(t, "andnot", body)
}

func TestTestExecutables_chmodBeforeWrite_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := `package fake
func f() {
	_ = os.Chmod(p, 0o755)
	_ = os.WriteFile(p, body, 0o644)
}
`
	assertExecRejected(t, "chmod_before", body)
}

func TestTestExecutables_chmodPlusX(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "chmod", `package fake
func f() {
	_ = os.WriteFile(path, body, 0o644)
	_ = os.Chmod(path, 0o111)
}
`)
}

func TestTestExecutables_allowsNonExecAndDirChmod(t *testing.T) {
	t.Parallel()
	assertExecAllowed(t, "644", `package fake
func f() { _ = os.WriteFile(path, body, 0o644) }
`)
	assertExecAllowed(t, "dirchmod", `package fake
func f() { _ = os.Chmod(opsDir, 0o755) }
`)
	assertExecAllowed(t, "andnot644", `package fake
func f() { _ = os.WriteFile(path, body, 0o755&^0o111) }
`)
}

func TestTestExecutables_crossFuncChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "cross_chmod", `package fake
func write() { _ = os.WriteFile(p, body, 0o644) }
func chmod() { _ = os.Chmod(p, 0o755) }
`)
	assertExecRejected(t, "cross_method", `package fake
func write() { _, _ = os.OpenFile(p, flag, 0o644) }
func chmod(f *os.File) { _ = f.Chmod(0o755) }
`)
}

func TestTestExecutables_unevaluableChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "uneval_chmod", `package fake
func write() { _ = os.WriteFile(p, body, 0o644) }
func chmod(mode os.FileMode) { _ = os.Chmod(p, mode) }
`)
}

func TestTestExecutables_crossFuncNonExecChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecAllowed(t, "cross_644", `package fake
func write() { _ = os.WriteFile(p, body, 0o644) }
func chmod() { _ = os.Chmod(p, 0o644) }
`)
}

func TestGitHermetic_repoguardNonAllowlistedInitFlagged_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"scripts/repoguard/sneaky_test.go": `package fake
func f() { _ = []string{"git", "init"} }
`,
	})
	_, errOut, code := runCheck(t, "git-hermetic", root)
	if code == 0 {
		t.Fatalf("expected fail on non-allowlisted scripts/repoguard init, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "gittest.Init") {
		t.Fatalf("must name gittest.Init, got %s", errOut)
	}
}

func TestGitHermetic_concatInitBypass_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := "package fake\nfunc f() { _ = []string{\"g\" + \"it\", \"init\"} }\n"
	if strings.Contains(body, `"git"`) {
		t.Fatal("fixture must not contain a quoted git token")
	}
	root := writeTree(t, map[string]string{
		"go.mod":                  "module fixture\n\ngo 1.22\n",
		"cmd/fake/concat_test.go": body,
	})
	_, errOut, code := runCheck(t, "git-hermetic", root)
	if code == 0 {
		t.Fatalf("expected fail on folded git init, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "gittest.Init") {
		t.Fatalf("must name gittest.Init, got %s", errOut)
	}
}

func TestGitHermetic_concatNameRequiresIsolationOrAllow(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                "module fixture\n\ngo 1.22\n",
		"cmd/fake/name_test.go": "package fake\nfunc f() { _ = \"g\" + \"it\" }\n",
	})
	_, errOut, code := runCheck(t, "git-hermetic", root)
	if code == 0 {
		t.Fatal("folded git name without allow must require isolation")
	}
	if !strings.Contains(errOut, "without TestMain isolation") {
		t.Fatalf("got %s", errOut)
	}
}

func TestGitHermetic_rawInitAndIsolation(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/x_test.go": `package fake
func f() { _ = []string{"git", "init"} }
`,
	})
	_, errOut, code := runCheck(t, "git-hermetic", root)
	if code == 0 {
		t.Fatal("expected fail")
	}
	if !strings.Contains(errOut, "gittest.Init") {
		t.Fatalf("got %s", errOut)
	}

	root = writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/status_test.go": `package fake
func f() { _ = exec.CommandContext(ctx, "git", "-C", dir, "status") }
`,
	})
	_, errOut, code = runCheck(t, "git-hermetic", root)
	if code == 0 {
		t.Fatal("expected isolation fail")
	}
	if !strings.Contains(errOut, "without TestMain isolation") {
		t.Fatalf("got %s", errOut)
	}

	root = writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/ok_test.go": `package fake
func TestMain(m *testing.M) { os.Exit(gittest.Main(m)) }
func f() { _ = exec.CommandContext(ctx, "git", "-C", dir, "status") }
`,
	})
	out, errOut, code := runCheck(t, "git-hermetic", root)
	if code != 0 {
		t.Fatalf("expected pass, stderr=%s", errOut)
	}
	if !strings.Contains(out, "OK: no raw git init") || !strings.Contains(out, "OK: every git-executing") {
		t.Fatalf("missing OK lines: %s", out)
	}
}

func TestRealRepoPassesGuards(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, check := range []string{"source-manifest", "test-executables", "git-hermetic"} {
		out, errOut, code := runCheck(t, check, root)
		if code != 0 {
			t.Fatalf("%s failed on real repo:\n%s\n%s", check, out, errOut)
		}
		if check == "git-hermetic" {
			if !strings.Contains(out, "internal/adapters/git_internal_test.go") {
				t.Fatalf("must list the git-name allow, got %s", out)
			}
			if !strings.Contains(out, "scripts/repoguard/repoguard_test.go") {
				t.Fatalf("must list the repoguard fixture allow, got %s", out)
			}
		}
	}
}

func assertExecRejected(t *testing.T, name, body string) {
	t.Helper()
	root := writeTree(t, map[string]string{
		"go.mod":                        "module fixture\n\ngo 1.22\n",
		"cmd/fake/" + name + "_test.go": body,
	})
	_, errOut, code := runCheck(t, "test-executables", root)
	if code == 0 {
		t.Fatalf("%s: expected fail", name)
	}
	if !strings.Contains(errOut, "testexec.Write") {
		t.Fatalf("%s: expected testexec.Write, got %s", name, errOut)
	}
}

func assertExecAllowed(t *testing.T, name, body string) {
	t.Helper()
	root := writeTree(t, map[string]string{
		"go.mod":                        "module fixture\n\ngo 1.22\n",
		"cmd/fake/" + name + "_test.go": body,
	})
	_, errOut, code := runCheck(t, "test-executables", root)
	if code != 0 {
		t.Fatalf("%s: expected pass, got %s", name, errOut)
	}
}

func runCheck(t *testing.T, check, root string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"-check", check, root}, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// legacy365ExactJSONTag is the #365 rg pattern: it matches json:"name" and
// misses json:"name,omitempty".
func legacy365ExactJSONTag(src, name string) bool {
	return strings.Contains(src, `json:"`+name+`"`)
}

// legacy368WriteFileMode is the #368 os.WriteFile/OpenFile pattern: a last
// octal digit in {1,3,5,7} on the same line. It misses 0o700/0o744/0o750 and
// named FileMode variables.
var legacy368WriteFileMode = regexp.MustCompile(`os\.(WriteFile|OpenFile)\([^;\n]*,\s*0o?[0-7]*[1357]\s*\)`)

// legacyIgnoreSubstring is the first typed-guard skip: any //go:build or
// // +build line containing the letters "ignore".
func legacyIgnoreSubstring(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "//go:build") || strings.HasPrefix(trim, "// +build") {
			return strings.Contains(trim, "ignore")
		}
		if strings.HasPrefix(trim, "//") {
			continue
		}
		return false
	}
	return false
}
