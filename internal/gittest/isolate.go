// Package gittest isolates Go tests from the host git environment and
// creates disposable repositories without depending on Git 2.28+ flags.
package gittest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const isolationMarker = "# armature-gittest-isolation\n"

const isolatedConfigBody = isolationMarker + `[user]
	name = Armature Test
	email = armature-test@example.com
[init]
	defaultBranch = main
	templateDir =
[commit]
	gpgsign = false
[tag]
	gpgsign = false
	forceSignAnnotated = false
[gc]
	auto = 0
	autoDetach = false
[maintenance]
	auto = false
[receive]
	autogc = false
[credential]
	helper =
[gpg]
	program = /bin/false
`

var gitOverrideEnvKeys = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
}

func IsolateGit() error {
	cfgPath, err := isolationConfigPath()
	if err != nil {
		return err
	}
	if err := os.Setenv("GIT_CONFIG_GLOBAL", cfgPath); err != nil {
		return fmt.Errorf("set GIT_CONFIG_GLOBAL: %w", err)
	}
	if err := os.Setenv("GIT_CONFIG_NOSYSTEM", "1"); err != nil {
		return fmt.Errorf("set GIT_CONFIG_NOSYSTEM: %w", err)
	}
	if err := os.Setenv("GIT_TERMINAL_PROMPT", "0"); err != nil {
		return fmt.Errorf("set GIT_TERMINAL_PROMPT: %w", err)
	}
	if err := os.Setenv("GIT_ASKPASS", "true"); err != nil {
		return fmt.Errorf("set GIT_ASKPASS: %w", err)
	}
	if err := os.Setenv("GIT_EDITOR", "true"); err != nil {
		return fmt.Errorf("set GIT_EDITOR: %w", err)
	}
	for _, key := range gitOverrideEnvKeys {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unset %s: %w", key, err)
		}
	}
	return nil
}

func isolationConfigPath() (string, error) {
	if existing := os.Getenv("GIT_CONFIG_GLOBAL"); existing != "" {
		data, err := os.ReadFile(existing) //nolint:gosec // G304: path is GIT_CONFIG_GLOBAL we wrote
		if err == nil && strings.HasPrefix(string(data), isolationMarker) {
			return existing, nil
		}
	}
	dir, err := os.MkdirTemp("", "armature-gittest-config-")
	if err != nil {
		return "", fmt.Errorf("mkdir git test config: %w", err)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(isolatedConfigBody), 0o600); err != nil {
		return "", fmt.Errorf("write git test config: %w", err)
	}
	return path, nil
}

func Main(m *testing.M) int {
	if err := IsolateGit(); err != nil {
		fmt.Fprintf(os.Stderr, "gittest.IsolateGit: %v\n", err)
		return 1
	}
	return m.Run()
}
