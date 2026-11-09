package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
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
func f() { _ = os.Chmod(opsDir, 0o000) }
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

func TestTestExecutables_paramPathHelperChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "param_path", `package fake
func write(p string) { _ = os.WriteFile(p, body, 0o644) }
func chmod(p string) { _ = os.Chmod(p, 0o755) }
func run(p string) {
	write(p)
	chmod(p)
}
`)
}

func TestTestExecutables_crossFilePkgPath_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/write_test.go": `package fake
var p string
func write() { _ = os.WriteFile(p, body, 0o644) }
`,
		"cmd/fake/chmod_test.go": `package fake
func chmod() { _ = os.Chmod(p, 0o755) }
`,
	})
	_, errOut, code := runCheck(t, "test-executables", root)
	if code == 0 {
		t.Fatal("expected fail on package-level path across files")
	}
	if !strings.Contains(errOut, "testexec.Write") {
		t.Fatalf("got %s", errOut)
	}
}

func TestTestExecutables_unrelatedParamChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "unrelated_params", `package fake
func w(p string) { _ = os.WriteFile(p, body, 0o600) }
func T(p string) { _ = os.Chmod(p, 0o755) }
`)
}

func TestTestExecutables_joinPathChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "join_paths", `package fake
func w() { _ = os.WriteFile(filepath.Join(dir, "tool"), body, 0o600) }
func T() { _ = os.Chmod(filepath.Join(dir, "tool"), 0o755) }
`)
}

func TestTestExecutables_pkgVarFoldedVsName_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/write_test.go": `package fake
var dest = "bin/tool"
func w() { _ = os.WriteFile(dest, body, 0o600) }
`,
		"cmd/fake/chmod_test.go": `package fake
func T() { _ = os.Chmod(dest, 0o755) }
`,
	})
	_, errOut, code := runCheck(t, "test-executables", root)
	if code == 0 {
		t.Fatal("expected fail on package-var chmod in a different file")
	}
	if !strings.Contains(errOut, "testexec.Write") {
		t.Fatalf("got %s", errOut)
	}
}

func TestTestExecutables_rawDirExecChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "raw_dir_755", `package fake
func f() { _ = os.Chmod(opsDir, 0o755) }
`)
}

func TestTestExecutables_localIdentDoesNotCollide_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecAllowed(t, "opsdir_locals", `package fake
func writeFileNotDir() {
	opsDir := "not-a-directory"
	_ = os.WriteFile(opsDir, body, 0o600)
}
func restoreDir() {
	opsDir := "ops"
	_ = os.Chmod(opsDir, 0o644)
}
`)
}

func TestTestExecutables_staleModeReassignParam_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "mode_reassign_param", `package fake
func f(m os.FileMode) {
	mode := os.FileMode(0o644)
	mode = m
	_ = os.Chmod(p, mode)
}
`)
}

func TestTestExecutables_staleModeReassignCall_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "mode_reassign_get", `package fake
func get() os.FileMode { return 0o755 }
func f() {
	mode := os.FileMode(0o644)
	mode = get()
	_ = os.WriteFile(p, nil, mode)
}
`)
}

func TestTestExecutables_closureReassignMode_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "closure_reassign", `package fake
func f() {
	mode := os.FileMode(0o644)
	func() { mode = 0o755 }()
	_ = os.Chmod(p, mode)
}
`)
}

func TestTestExecutables_addrTakenMode_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "addr_taken", `package fake
func set(m *os.FileMode) { *m = 0o755 }
func f() {
	mode := os.FileMode(0o644)
	set(&mode)
	_ = os.Chmod(p, mode)
}
`)
}

func TestTestExecutables_ioutilWriteFileExec_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	assertExecRejected(t, "ioutil_755", `package fake
func f() { _ = ioutil.WriteFile(p, nil, 0o755) }
`)
}

func TestTestExecutables_syscallChmodNonExec(t *testing.T) {
	t.Parallel()
	assertExecAllowed(t, "syscall_644", `package fake
func f() { _ = syscall.Chmod(p, 0o644) }
`)
}

func TestTestExecutables_exemptionDoesNotMaskSneakyChmod_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/armature/helpers_test.go": `package fake
func restore() {
	opsDir := filepath.Join(repo, "ops")
	_ = os.Chmod(opsDir, 0o755)
}
func sneaky() {
	_ = os.WriteFile(p, body, 0o600)
	_ = os.Chmod(p, 0o755)
}
`,
	})
	_, errOut, code := runCheck(t, "test-executables", root)
	if code == 0 {
		t.Fatalf("sneaky write+chmod must not be masked, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "testexec.Write") {
		t.Fatalf("got %s", errOut)
	}
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
	out, errOut, code := runCheckAllows(t, "git-hermetic", root, nil)
	if code != 0 {
		t.Fatalf("expected pass, stderr=%s", errOut)
	}
	if !strings.Contains(out, "OK: no raw git init") || !strings.Contains(out, "OK: every git-executing") {
		t.Fatalf("missing OK lines: %s", out)
	}
}

func TestGolangciInTests_execFixtureFails_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/lint_test.go": `package fake
import (
	"context"
	"os/exec"
)
func TestBad(t *testing.T) {
	_ = exec.Command("golangci-lint", "run")
	_ = exec.CommandContext(context.Background(), "golangci-lint", "run")
	_, _ = exec.LookPath("golangci-lint")
}
`,
	})
	_, errOut, code := runCheck(t, "golangci-in-tests", root)
	if code == 0 {
		t.Fatalf("expected fail on golangci-lint exec in _test.go, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "golangci-lint") {
		t.Fatalf("must name golangci-lint, got %s", errOut)
	}
	if !strings.Contains(errOut, "gochecknoglobals-probe") {
		t.Fatalf("must point at gochecknoglobals-probe, got %s", errOut)
	}
}

func TestGolangciInTests_foldedNameFails_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	body := "package fake\nfunc f() { _ = exec.Command(\"golangci-\"+\"lint\", \"run\") }\n"
	if strings.Contains(body, `"golangci-lint"`) {
		t.Fatal("fixture must not contain a quoted golangci-lint token")
	}
	root := writeTree(t, map[string]string{
		"go.mod":                  "module fixture\n\ngo 1.22\n",
		"cmd/fake/folded_test.go": body,
	})
	_, errOut, code := runCheck(t, "golangci-in-tests", root)
	if code == 0 {
		t.Fatalf("expected fail on folded golangci-lint name, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "execs golangci-lint") {
		t.Fatalf("got %s", errOut)
	}
}

func TestGolangciInTests_allowsMentionWithoutExec(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/mention_test.go": `package fake
func TestDoc(t *testing.T) {
	_ = "golangci-lint is run by make lint / repoguard, not go test"
}
`,
	})
	out, errOut, code := runCheck(t, "golangci-in-tests", root)
	if code != 0 {
		t.Fatalf("expected pass, stderr=%s", errOut)
	}
	if !strings.Contains(out, "OK: no _test.go execs golangci-lint") {
		t.Fatalf("missing OK line: %s", out)
	}
}

func TestGochecknoglobalsProbe_selftestPasses_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	out, errOut, code := runCheck(t, "gochecknoglobals-probe", root)
	if code != 0 {
		t.Fatalf("gochecknoglobals-probe failed on real repo:\n%s\n%s", out, errOut)
	}
	if !strings.Contains(out, "OK: gochecknoglobals rejects reintroduced mutable requireOwnerHook") {
		t.Fatalf("missing OK line: %s", out)
	}
}

func TestGochecknoglobalsProbe_missingEnableFails(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":        "module fixture\n\ngo 1.22\n",
		".golangci.yml": "version: \"2\"\nlinters:\n  default: none\n  enable:\n    - errcheck\n",
	})
	_, errOut, code := runCheck(t, "gochecknoglobals-probe", root)
	if code == 0 {
		t.Fatal("expected fail when gochecknoglobals is not enabled")
	}
	if !strings.Contains(errOut, "gochecknoglobals") {
		t.Fatalf("got %s", errOut)
	}
}

func TestRealRepoPassesGuards(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	checks := []string{
		"source-manifest", "test-executables", "git-hermetic", "gochecknoglobals-probe",
		"golangci-in-tests", "object-id", "residual-trls", "git-network",
	}
	for _, check := range checks {
		out, errOut, code := runCheck(t, check, root)
		if code != 0 {
			t.Fatalf("%s failed on real repo:\n%s\n%s", check, out, errOut)
		}
		if check == "git-hermetic" {
			if !strings.Contains(out, "scripts/repoguard/repoguard_test.go") {
				t.Fatalf("must list the repoguard fixture allow, got %s", out)
			}
		}
		if check == "object-id" && !strings.Contains(out, "OK: object-id") {
			t.Fatalf("missing object-id OK line: %s", out)
		}
		if check == "residual-trls" && !strings.Contains(out, "OK: residual trls") {
			t.Fatalf("missing residual-trls OK line: %s", out)
		}
		if check == "git-network" {
			if !strings.Contains(out, "OK: remote-network git calls go through networkCmd") {
				t.Fatalf("missing git-network OK line: %s", out)
			}
			if !strings.Contains(out, "bundle-create-local-bare-push") || !strings.Contains(out, "e2e-local-origin-clone") {
				t.Fatalf("must list per-site local-transport allows, got %s", out)
			}
		}
	}
}

func TestObjectID_FortyOnlyPatternFails_REQ_PV70(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
import "regexp"
var pat = regexp.MustCompile(` + "`^[a-f0-9]{40}$`" + `)
`,
		"docs/schemas/review-bundle.schema.json": `{
  "properties": {
    "delivery": {
      "properties": {
        "base_sha": { "pattern": "^[a-f0-9]{40}$" }
      }
    }
  }
}
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on 40-hex-only patterns, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "gitoid") {
		t.Fatalf("fix must name gitoid, got %s", errOut)
	}
}

func TestObjectID_BothWidthsPass_REQ_PV70(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const Pattern = ` + "`^[a-f0-9]{40}$|^[a-f0-9]{64}$`" + `
`,
		"docs/schemas/ok.schema.json": `{"properties":{"sha":{"pattern":"^[a-f0-9]{40}$|^[a-f0-9]{64}$"}}}`,
	})
	out, errOut, code := runCheckAllows(t, "object-id", root, nil)
	if code != 0 {
		t.Fatalf("expected pass, stdout=%s stderr=%s", out, errOut)
	}
}

func TestGitNetwork_testdataBypasses_REQ_REPOGUARD(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
		verb string
	}{
		{"fetch", "fetch_origin.go", "fetch"},
		{"push", "push_origin.go", "push"},
		{"ls-remote", "ls_remote_commandgit.go", "ls-remote"},
		{"variable", "var_subcommand.go", "fetch"},
		{"const", "const_subcommand.go", "pull"},
		{"append", "append_args.go", "push"},
		{"alias", "alias_builder.go", "clone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := readGitNetFixture(t, tc.file)
			root := writeTree(t, map[string]string{
				"go.mod":             "module fixture\n\ngo 1.22\n",
				"cmd/fake/bypass.go": body,
			})
			_, errOut, code := runCheck(t, "git-network", root)
			if code == 0 {
				t.Fatalf("%s: expected fail", tc.name)
			}
			if !strings.Contains(errOut, "outside networkCmd") || !strings.Contains(errOut, tc.verb) {
				t.Fatalf("%s: must name networkCmd and %s, got %s", tc.name, tc.verb, errOut)
			}
			if !strings.Contains(errOut, "Use networkCmd") {
				t.Fatalf("%s: fix message missing, got %s", tc.name, errOut)
			}
		})
	}
}

func TestGitNetwork_sanctionedAndLocalAllowPass(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                   "module fixture\n\ngo 1.22\n",
		"cmd/fake/ok.go":           readGitNetFixture(t, "sanctioned_networkcmd.go"),
		"internal/adapters/git.go": readGitNetFixture(t, "local_transport_allow.go"),
	})
	out, errOut, code := runCheck(t, "git-network", root)
	if code != 0 {
		t.Fatalf("expected pass, stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "OK: remote-network git calls go through networkCmd") {
		t.Fatalf("missing OK line: %s", out)
	}
	if !strings.Contains(out, "bundle-create-local-bare-push") {
		t.Fatalf("must list the local-transport allow, got %s", out)
	}
}

func TestGitNetwork_fileCommentDoesNotAllow(t *testing.T) {
	t.Parallel()
	body := `// gitnetallow: bundle-create-local-bare-push local bare path under armature-tmp-work, not a named remote
package fake

func bad(c *Client) {
	c.cmd("fetch", "origin", "main")
}
`
	root := writeTree(t, map[string]string{
		"go.mod":           "module fixture\n\ngo 1.22\n",
		"cmd/fake/wide.go": body,
	})
	_, errOut, code := runCheck(t, "git-network", root)
	if code == 0 {
		t.Fatal("file-level gitnetallow must not exempt a fetch")
	}
	if !strings.Contains(errOut, "outside networkCmd") {
		t.Fatalf("got %s", errOut)
	}
}

func readGitNetFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "scripts", "repoguard", "testdata", "gitnet", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestObjectID_Xdigit40Fails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/zz/a.go": `package zz
var A = "^[[:xdigit:]]{40}$"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on [[:xdigit:]]{40}, stderr=%s", errOut)
	}
}

func TestObjectID_UnanchoredHex40Fails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "[0-9a-f]{40}"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on unanchored [0-9a-f]{40}, stderr=%s", errOut)
	}
}

func TestObjectID_FortyHexBasicLitFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const sample = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on 40-hex BasicLit, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "BasicLit") && !strings.Contains(errOut, "gitoid") {
		t.Fatalf("must name BasicLit or gitoid, got %s", errOut)
	}
}

func TestObjectID_LenLessThan40Fails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
func ok(s string) bool { return len(s) < 40 }
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on len(x) < 40, stderr=%s", errOut)
	}
}

func TestObjectID_HexLenConstFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const HexLen = 40
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on const HexLen = 40, stderr=%s", errOut)
	}
}

func TestObjectID_UncompilablePatternFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "^[0-9a-f]{40}$|bogus("
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on uncompilable OID pattern, stderr=%s", errOut)
	}
}

func TestObjectID_FindStringPartialFails_REQ_419(t *testing.T) {
	t.Parallel()
	// Unanchored {40} matches a prefix of 64-hex via FindString.
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "[0-9a-f]{40}"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on FindString(64)!=64 probe, stderr=%s", errOut)
	}
	if !oidPatternBanned(`[0-9a-f]{40}`) {
		t.Fatal("FindString probe mutant: unanchored {40} must be banned")
	}
	if oidPatternBanned(`^[a-f0-9]{40}$|^[a-f0-9]{64}$`) {
		t.Fatal("dual-width PatternLower must not be flagged")
	}
}

func TestObjectID_Bare64SubstringDoesNotSuppress_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const note = ` + "`oid [0-9a-f]{40} (also see {64} elsewhere)`" + `
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail: bare {64} must not suppress 40-only hit, stderr=%s", errOut)
	}
}

func TestObjectID_Alnum40Fails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/zz/a.go": `package zz
var A = "^[[:alnum:]]{40}$"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on [[:alnum:]]{40}, stderr=%s", errOut)
	}
}

func TestObjectID_Brace40Comma40Fails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "^[0-9a-f]{40,40}$"
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on {40,40}, stderr=%s", errOut)
	}
}

func TestObjectID_Word40AnchoredFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const pat = ` + "`^\\w{40}$`" + `
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on ^\\w{40}$, stderr=%s", errOut)
	}
}

func TestObjectID_LenEq40Kept_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
func ok(s string) bool { return len(s) == 40 }
`,
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on len==40, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "gitoid") {
		t.Fatalf("fix must name gitoid, got %s", errOut)
	}
}

func TestObjectID_TextGrepQEFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":               "module fixture\n\ngo 1.22\n",
		"scripts/check_sha.sh": "grep -qE '^[0-9a-f]{40}$' \"$1\"\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on grep -qE 40-only, stderr=%s", errOut)
	}
}

func TestObjectID_TextReAssignFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":               "module fixture\n\ngo 1.22\n",
		"scripts/check_sha.sh": "re='^[0-9a-f]{40}$'\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on re= assign, stderr=%s", errOut)
	}
}

func TestObjectID_TextBREFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":         "module fixture\n\ngo 1.22\n",
		"scripts/bre.sh": "grep '^[0-9a-f]\\{40\\}$' \"$1\"\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on BRE \\{40\\}, stderr=%s", errOut)
	}
}

func TestObjectID_TextAwkFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":       "module fixture\n\ngo 1.22\n",
		"scripts/a.sh": "awk '/^[0-9a-f]{40}$/' \"$1\"\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on awk /{40}/, stderr=%s", errOut)
	}
}

func TestObjectID_TextPyHeredocFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":         "module fixture\n\ngo 1.22\n",
		"scripts/pin.sh": "python3 - <<'PY'\nimport re\nre.fullmatch(r'^[0-9a-f]{40}$', s)\nPY\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on re.fullmatch in heredoc, stderr=%s", errOut)
	}
}

func TestObjectID_TextBashRegexFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":        "module fixture\n\ngo 1.22\n",
		"scripts/eq.sh": "[[ $x =~ ^[0-9a-f]{40}$ ]]\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on unquoted =~, stderr=%s", errOut)
	}
}

func TestObjectID_TextPyConstantFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":          "module fixture\n\ngo 1.22\n",
		"scripts/oids.py": "SHA_RE = r'^[0-9a-f]{40}$'\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on plain .py constant, stderr=%s", errOut)
	}
}

func TestObjectID_TextWorkflowYAMLFails_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                  "module fixture\n\ngo 1.22\n",
		".github/workflows/x.yml": "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: grep -E '^[0-9a-f]{40}$' file\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on workflow YAML grep -E, stderr=%s", errOut)
	}
}

func TestObjectID_ProbeWeakenedFailsREQ_419(t *testing.T) {
	t.Parallel()
	if !oidPatternBanned(`^[[:xdigit:]]{40}$`) {
		t.Fatal("weaken detection: compile-probe must reject [[:xdigit:]]{40}")
	}
	if oidPatternBanned(`^[a-f0-9]{40}$|^[a-f0-9]{64}$`) {
		t.Fatal("dual-width PatternLower must not be flagged")
	}
	if !oidPatternBanned(`^[0-9a-f]{40}$|bogus(`) {
		t.Fatal("compile-error mutant: uncompilable OID pattern must be a hit")
	}
}

func TestObjectID_WaiverLineDoesNotCoverNewHit_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"scripts/test_check_jobs.sh": strings.Repeat("\n", 374) +
			`sha_re = re.compile(r"^[0-9a-f]{40}$")` + "\n" +
			`other = re.compile(r"^[[:xdigit:]]{40}$")` + "\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("expected fail on new pattern in waived file, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "test_check_jobs.sh:376") && !strings.Contains(errOut, "[[:xdigit:]]") {
		t.Fatalf("must flag the new line, not only the waived one; stderr=%s", errOut)
	}
}

func TestObjectID_AppendGrepToWaivedFileFails_REQ_419(t *testing.T) {
	t.Parallel()
	// Per-site waiver at line 375 must not cover an appended plant.
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "test_check_jobs.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, map[string]string{
		"go.mod":                     "module fixture\n\ngo 1.22\n",
		"scripts/test_check_jobs.sh": string(body) + "\ngrep -qE '^[0-9a-f]{40}$' \"$1\"\n",
	})
	_, errOut, code := runCheck(t, "object-id", root)
	if code == 0 {
		t.Fatalf("appending grep -qE 40-only to waived file must FAIL, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "test_check_jobs.sh") {
		t.Fatalf("must name waived file hit, got %s", errOut)
	}
}

func TestResidualTrls_CmdGoFails_REQ_421(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/armature/main.go": `package main
const brand = "trls tui"
`,
	})
	_, errOut, code := runCheck(t, "residual-trls", root)
	if code == 0 {
		t.Fatalf("expected fail on cmd/ trls, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "cmd/armature/main.go") {
		t.Fatalf("must name cmd hit, got %s", errOut)
	}
}

func TestResidualTrls_TestGoFails_REQ_421(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/foo/foo_test.go": `package foo
func TestX(t *testing.T) { _ = "run trls ready" }
`,
	})
	_, errOut, code := runCheck(t, "residual-trls", root)
	if code == 0 {
		t.Fatalf("expected fail on _test.go trls, stderr=%s", errOut)
	}
}

func TestResidualTrls_CastFails_REQ_421(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                "module fixture\n\ngo 1.22\n",
		"docs/assets/demo.cast": "{\"version\":2}\n[0.0, \"o\", \"trls tui · worker\"]\n",
	})
	_, errOut, code := runCheck(t, "residual-trls", root)
	if code == 0 {
		t.Fatalf("expected fail on docs/assets cast, stderr=%s", errOut)
	}
}

func TestResidualTrls_CleanPasses_REQ_421(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                   "module fixture\n\ngo 1.22\n",
		"cmd/armature/main.go":     "package main\nconst brand = \"arm tui\"\n",
		"internal/foo/foo_test.go": "package foo\nfunc TestX(t *testing.T) { _ = \"arm ready\" }\n",
		"docs/assets/demo.cast":    "{\"version\":2}\n[0.0, \"o\", \"arm tui · worker\"]\n",
	})
	out, errOut, code := runCheckAllows(t, "residual-trls", root, nil)
	if code != 0 {
		t.Fatalf("expected pass, stdout=%s stderr=%s", out, errOut)
	}
}

func TestResidualTrls_WaiverLineDoesNotCoverNewHit_REQ_421(t *testing.T) {
	t.Parallel()
	// Restores the parallel coverage lost when the worker-package grep guard
	// moved into repoguard: a new trls on a non-waived line must FAIL.
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/worker/residual_trls_test.go": strings.Repeat("\n", 16) +
			`assert.NotContains(t, err.Error(), "trls")` + "\n" +
			`// trls new` + "\n",
		"internal/tui/app/model_test.go": strings.Repeat("\n", 75) +
			`if strings.Contains(nav, "trls") {` + "\n" +
			`t.Errorf("nav must not retain pre-rename trls brand, got: %q", nav)` + "\n",
	})
	_, errOut, code := runCheck(t, "residual-trls", root)
	if code == 0 {
		t.Fatalf("expected fail on // trls new after waived line, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "residual_trls_test.go:18") && !strings.Contains(errOut, "trls new") {
		t.Fatalf("must flag the new line, got %s", errOut)
	}
}

func TestResidualTrls_RejectsLineZeroWaiver_REQ_421(t *testing.T) {
	t.Parallel()
	if allowedResidualTrlsSite("internal/worker/residual_trls_test.go", 0, "trls") {
		t.Fatal("Line == 0 must not waive residual-trls")
	}
	if !allowedResidualTrlsSite("internal/worker/residual_trls_test.go", 17, "trls") {
		t.Fatal("line 17 + needle must waive")
	}
}

// --- Fail-closed plants (#424): each plant fails rc!=0 against the real binary. ---

func realRepoguard(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "repoguard")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(repoRoot(t), "scripts", "repoguard")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build repoguard: %v\n%s", err, out)
	}
	return bin
}

func runRealObjectID(t *testing.T, root string) (stderr string, code int) {
	t.Helper()
	bin := realRepoguard(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-check", "object-id", root)
	var errBuf bytes.Buffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run repoguard: %v", err)
		}
	}
	return errBuf.String(), code
}

func TestPlant_ConstN40_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const n = 40
`,
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant const n = 40 must fail, stderr=%s", errOut)
	}
	t.Logf("plant const n=40 rc=%d", code)
}

func TestPlant_Hex64BasicLit_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
const sample = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
`,
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant 64-hex BasicLit must fail, stderr=%s", errOut)
	}
	t.Logf("plant 64-hex BasicLit rc=%d", code)
}

func TestPlant_Pattern64Only_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "^[0-9a-f]{64}$"
`,
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant 64-only pattern must fail, stderr=%s", errOut)
	}
	t.Logf("plant ^[0-9a-f]{64}$ rc=%d", code)
}

func TestPlant_UncompilableRegexp_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"internal/fake/oid.go": `package fake
var pat = "([0-9a-f]+"
`,
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant uncompilable ([0-9a-f]+ must fail, stderr=%s", errOut)
	}
	t.Logf("plant uncompilable regexp rc=%d", code)
}

func TestPlant_UnterminatedQuote_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":            "module fixture\n\ngo 1.22\n",
		"scripts/broken.sh": "#!/bin/bash\necho 'unterminated\n",
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant unterminated quote must fail, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "tokenizer fail-closed") && !strings.Contains(errOut, "unterminated quote") {
		t.Fatalf("must name tokenizer/unterminated, got %s", errOut)
	}
	t.Logf("plant unterminated quote rc=%d", code)
}

func TestPlant_UnquotedGrep40_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":            "module fixture\n\ngo 1.22\n",
		"scripts/grep40.sh": "grep -E ^[0-9a-f]{40}$ \"$1\"\n",
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant unquoted grep -E 40-only must fail, stderr=%s", errOut)
	}
	t.Logf("plant unquoted grep -E rc=%d", code)
}

func TestPlant_YAMLPattern40_REQ_419(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":              "module fixture\n\ngo 1.22\n",
		".github/pattern.yml": "pattern: ^[0-9a-f]{40}$\n",
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("plant YAML pattern 40-only must fail, stderr=%s", errOut)
	}
	t.Logf("plant YAML pattern rc=%d", code)
}

func TestPlant_TextSkipDirsExplicit_REQ_419(t *testing.T) {
	t.Parallel()
	// Explicit justified list: docs/, ops/, non-.github dot-dirs skipped; else scanned.
	want := map[string]bool{"testdata": true, "vendor": true, ".git": true, "node_modules": true, "ops": true, "docs": true}
	got := textSkipDirList()
	if len(got) != len(want) {
		t.Fatalf("textSkipDirList=%v want exactly %v", got, want)
	}
	for _, d := range got {
		if !want[d] {
			t.Fatalf("unexpected skip %q; justified list is %v", d, want)
		}
	}
	root := writeTree(t, map[string]string{
		"go.mod":                   "module fixture\n\ngo 1.22\n",
		"docs/hidden.sh":           "grep -E ^[0-9a-f]{40}$ x\n",
		"ops/hidden.sh":            "grep -E ^[0-9a-f]{40}$ x\n",
		".cache/hidden.sh":         "grep -E ^[0-9a-f]{40}$ x\n",
		"scripts/visible.sh":       "grep -E ^[0-9a-f]{40}$ x\n",
		".github/workflows/ci.yml": "run: grep -E '^[0-9a-f]{40}$' x\n",
	})
	errOut, code := runRealObjectID(t, root)
	if code == 0 {
		t.Fatalf("visible scripts/.github must still be scanned, stderr=%s", errOut)
	}
	if strings.Contains(errOut, "docs/hidden") || strings.Contains(errOut, "ops/hidden") || strings.Contains(errOut, ".cache/hidden") {
		t.Fatalf("docs/ops/dot-dirs must be skipped, got %s", errOut)
	}
	if !strings.Contains(errOut, "scripts/visible") && !strings.Contains(errOut, ".github/workflows") {
		t.Fatalf("must flag scanned paths, got %s", errOut)
	}
	t.Logf("plant textSkipDirs scan/skip rc=%d", code)
}

func TestValidateAllowEntries_LineZeroFails_REQ_419(t *testing.T) {
	t.Parallel()
	err := validateAllowEntries([]allowEntry{{
		RelPath: "internal/fake/plant.go",
		Check:   allowObjectID,
		Line:    0,
		Needle:  "plant",
		Reason:  "planted Line==0 must fail",
	}})
	if err == nil {
		t.Fatal("validateAllowEntries must reject Line==0")
	}
	if !strings.Contains(err.Error(), "Line == 0") {
		t.Fatalf("must name Line == 0, got %v", err)
	}
}

func TestPlant_EmptyNeedleFails_REQ_419(t *testing.T) {
	t.Parallel()
	err := validateAllowEntries([]allowEntry{{
		RelPath: "go.mod",
		Check:   allowObjectID,
		Line:    1,
		Needle:  "",
		Reason:  "empty needle plant",
	}})
	code := 0
	if err != nil {
		code = 1
	}
	if code == 0 {
		t.Fatal("empty Needle must FAIL validateAllowEntries")
	}
	if !strings.Contains(err.Error(), "empty Needle") {
		t.Fatalf("must name empty Needle, got %v", err)
	}
	// Empty needle must not waive any hit at file:line.
	idx := newAllowIndex([]allowEntry{{
		RelPath: "go.mod",
		Check:   allowObjectID,
		Line:    1,
		Needle:  "",
		Reason:  "empty needle plant",
	}})
	if idx.allowed("go.mod", allowObjectID, 1, "module github.com/scullxbones/armature") {
		t.Fatal("empty Needle must not match any hit")
	}
	t.Logf("plant empty-needle rc=%d", code)
}

func TestPlant_UnusedNoHitFails_REQ_419(t *testing.T) {
	t.Parallel()
	// Needle text is on go.mod:1 ("module") but never waives an object-id hit.
	entries := []allowEntry{{
		RelPath: "go.mod",
		Check:   allowObjectID,
		Line:    1,
		Needle:  "module",
		Reason:  "no-hit plant",
	}}
	if err := validateAllowEntries(entries); err != nil {
		t.Fatalf("structural validate must pass: %v", err)
	}
	idx := newAllowIndex(entries)
	// No idx.allowed calls → unused.
	err := idx.unused(allowObjectID)
	code := 0
	if err != nil {
		code = 1
	}
	if code == 0 {
		t.Fatal("entry that never waived a hit must FAIL unused")
	}
	if !strings.Contains(err.Error(), "unused") {
		t.Fatalf("must name unused, got %v", err)
	}
	t.Logf("plant no-hit unused rc=%d", code)
}

func TestPlant_MissingFileUnusedFails_REQ_419(t *testing.T) {
	t.Parallel()
	entries := []allowEntry{{
		RelPath: "does-not-exist-plant.go",
		Check:   allowObjectID,
		Line:    1,
		Needle:  `^[0-9a-f]{40}$`,
		Reason:  "missing-file plant",
	}}
	if err := validateAllowEntries(entries); err != nil {
		t.Fatalf("structural validate must pass: %v", err)
	}
	idx := newAllowIndex(entries)
	err := idx.unused(allowObjectID)
	code := 0
	if err != nil {
		code = 1
	}
	if code == 0 {
		t.Fatal("missing-file entry must FAIL as unused")
	}
	if !strings.Contains(err.Error(), "unused") || !strings.Contains(err.Error(), "does-not-exist-plant.go") {
		t.Fatalf("must name unused + missing path, got %v", err)
	}
	t.Logf("plant missing-file unused rc=%d", code)
}

func TestPlant_StaleNeedleUnusedFails_REQ_419(t *testing.T) {
	t.Parallel()
	// Needle absent from line → never matches a hit → unused FAIL
	// (replaces validateAllowEntriesPresent ErrNotExist/stale skip).
	entries := []allowEntry{{
		RelPath: "waived.sh",
		Check:   allowObjectID,
		Line:    1,
		Needle:  `^[0-9a-f]{40}$`,
		Reason:  "stale plant",
	}}
	idx := newAllowIndex(entries)
	if idx.allowed("waived.sh", allowObjectID, 1, "echo ok") {
		t.Fatal("stale needle must not match")
	}
	err := idx.unused(allowObjectID)
	code := 0
	if err != nil {
		code = 1
	}
	if code == 0 {
		t.Fatal("stale needle (no hit waived) must FAIL unused")
	}
	t.Logf("plant stale-needle unused rc=%d", code)
}

func TestPlant_GitHermeticRejectsWholeFile_REQ_419(t *testing.T) {
	t.Parallel()
	// Avoid a bare "git" BasicLit here — that would mark package main as
	// git-executing and fail TestRealRepoPassesGuards (no TestMain).
	gitNeedle := string([]byte{'g', 'i', 't'})
	if allowedGitHermeticSite("internal/adapters/git_internal_test.go", allowGitHermeticName, 0, gitNeedle) {
		t.Fatal("git-hermetic allowed() must reject Line==0 whole-file")
	}
	if allowedSite("any.go", allowObjectID, 0, "x") {
		t.Fatal("shared allowedSite must reject Line==0")
	}
	if err := validateAllowEntries([]allowEntry{{
		RelPath: "x.go", Check: allowGitHermeticName, Line: 0, Needle: gitNeedle, Reason: "plant",
	}}); err == nil {
		t.Fatal("shared validator must reject Line==0 for git-hermetic")
	}
}

func TestUnusedWaiver_NonArmatureModuleFails_REQ_437(t *testing.T) {
	t.Parallel()
	// Fixture go.mod is not github.com/scullxbones/armature — unused audit
	// must still run (no isArmatureRoot gate).
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
	})
	allows := []allowEntry{{
		RelPath: "does-not-exist-437.go",
		Check:   allowObjectID,
		Line:    1,
		Needle:  `^[0-9a-f]{40}$`,
		Reason:  "unused waiver on non-armature module plant",
	}}
	_, errOut, code := runCheckAllows(t, "object-id", root, allows)
	if code != 1 {
		t.Fatalf("unused waiver on module fixture must FAIL rc=1, got %d stderr=%s", code, errOut)
	}
	if !strings.Contains(errOut, "unused") || !strings.Contains(errOut, "does-not-exist-437.go") {
		t.Fatalf("must name unused + path, got %s", errOut)
	}
}

func TestUnusedWaiver_RealRepoPlantsFail_REQ_437(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	withPlant := func(plant allowEntry) []allowEntry {
		return append(append([]allowEntry(nil), allowlist()...), plant)
	}

	_, errOut, code := runCheckAllows(t, "object-id", root, withPlant(allowEntry{
		RelPath: "go.mod", Check: allowObjectID, Line: 1, Needle: "", Reason: "empty-needle plant",
	}))
	if code != 1 || !strings.Contains(errOut, "empty Needle") {
		t.Fatalf("empty-needle must FAIL on real repo, rc=%d stderr=%s", code, errOut)
	}

	_, errOut, code = runCheckAllows(t, "object-id", root, withPlant(allowEntry{
		RelPath: "go.mod", Check: allowObjectID, Line: 1, Needle: "module", Reason: "no-hit plant",
	}))
	if code != 1 || !strings.Contains(errOut, "unused") {
		t.Fatalf("no-hit waiver must FAIL unused on real repo, rc=%d stderr=%s", code, errOut)
	}

	_, errOut, code = runCheckAllows(t, "object-id", root, withPlant(allowEntry{
		RelPath: "missing-437-plant.go", Check: allowObjectID, Line: 1,
		Needle: `^[0-9a-f]{40}$`, Reason: "missing-file plant",
	}))
	if code != 1 || !strings.Contains(errOut, "unused") || !strings.Contains(errOut, "missing-437-plant.go") {
		t.Fatalf("missing-file must FAIL unused on real repo, rc=%d stderr=%s", code, errOut)
	}

	// Shifted line: real waived site at scripts/test_check_jobs.sh:466, off-by-one.
	_, errOut, code = runCheckAllows(t, "object-id", root, withPlant(allowEntry{
		RelPath: "scripts/test_check_jobs.sh", Check: allowObjectID, Line: 467,
		Needle: `^[0-9a-f]{40}$`, Reason: "shifted-line plant",
	}))
	if code != 1 || !strings.Contains(errOut, "unused") {
		t.Fatalf("shifted-line waiver must FAIL unused on real repo, rc=%d stderr=%s", code, errOut)
	}
}

func TestGitHermetic_FailPrintsNoOK_REQ_437(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module fixture\n\ngo 1.22\n",
		"cmd/fake/x_test.go": `package fake
func f() { _ = []string{"git", "init"} }
`,
	})
	out, errOut, code := runCheckAllows(t, "git-hermetic", root, nil)
	if code == 0 {
		t.Fatalf("expected fail, stderr=%s", errOut)
	}
	if strings.Contains(out, "OK:") {
		t.Fatalf("failing git-hermetic must print no OK line, stdout=%s", out)
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

// runCheckAllows invokes one allowlist-aware check with an explicit allowlist
// (nil/empty for fixture scans that must not inherit production waivers).
func runCheckAllows(t *testing.T, check, root string, allows []allowEntry) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	var err error
	switch check {
	case "object-id":
		err = checkObjectID(root, allows, &stdout, &stderr)
	case "git-hermetic":
		err = checkGitHermetic(root, allows, &stdout, &stderr)
	case "residual-trls":
		err = checkResidualTrls(root, allows, &stdout, &stderr)
	default:
		t.Fatalf("runCheckAllows: unsupported check %q", check)
	}
	code := 0
	if err != nil {
		code = 1
	}
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
