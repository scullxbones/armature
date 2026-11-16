package adapters_test

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubActionsAuthScript_NoTokenLeak_REQ_481(t *testing.T) {
	t.Parallel()
	probe := "ghs_SCRIPT_PROBE_481!"
	script := filepath.Join(moduleRoot(t), "scripts", "github-actions-git-auth.sh")

	trace := exec.Command("bash", "-x", "-c", `source "$1"; github_actions_git_auth_export`, "bash", script)
	trace.Env = append(os.Environ(), "GITHUB_TOKEN="+probe)
	traced, err := trace.CombinedOutput()
	if err != nil {
		t.Fatalf("auth export under bash -x: %v", err)
	}
	if strings.Contains(string(traced), probe) {
		t.Fatal("token leaked into bash -x trace")
	}

	show := exec.Command("bash", "-c", `
set -euo pipefail
export GIT_CONFIG_COUNT=1
export GIT_CONFIG_KEY_0=user.name
export GIT_CONFIG_VALUE_0=inherited-probe
source "$1"
github_actions_git_auth_export
printf 'COUNT=%s\n' "$GIT_CONFIG_COUNT"
printf 'KEY0=%s\n' "$GIT_CONFIG_KEY_0"
printf 'VAL0=%s\n' "$GIT_CONFIG_VALUE_0"
printf 'KEY1=%s\n' "$GIT_CONFIG_KEY_1"
printf 'VAL1=%s\n' "$GIT_CONFIG_VALUE_1"
`, "bash", script)
	show.Env = append(os.Environ(), "GITHUB_TOKEN="+probe)
	out, err := show.CombinedOutput()
	if err != nil {
		t.Fatalf("auth export: %v", err)
	}
	text := string(out)
	if strings.Contains(text, probe) || strings.Contains(text, "AUTHORIZATION: bearer") || strings.Contains(text, "AUTHORIZATION: Bearer") {
		t.Fatal("raw token or bearer header present in auth script output")
	}
	if !strings.Contains(text, "COUNT=2") || !strings.Contains(text, "KEY0=user.name") || !strings.Contains(text, "VAL0=inherited-probe") {
		t.Fatalf("inherited GIT_CONFIG slot not preserved:\n%s", text)
	}
	if !strings.Contains(text, "KEY1=http.https://github.com/.extraHeader") {
		t.Fatalf("auth key not appended at index 1:\n%s", text)
	}
	const prefix = "VAL1=AUTHORIZATION: basic "
	var encoded string
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			encoded = rest
		}
	}
	raw, decErr := base64.StdEncoding.DecodeString(encoded)
	if decErr != nil || string(raw) != "x-access-token:"+probe {
		t.Fatal("script header did not decode to x-access-token plus the job token")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
