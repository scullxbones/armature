package output

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func checkShellCompletion(body []byte) error {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("artifact fixture is empty")
	}
	switch {
	case isPowerShellCompletion(text):
		return nil
	case isZshCompletion(text):
		return nil
	case isFishCompletion(text):
		return nil
	case isBashCompletion(text):
		return checkBashSyntax(text)
	default:
		return fmt.Errorf("completion artifact must match bash, zsh, fish, or powershell completion-script grammar (cited by %s)", CitationShellCompletionGrammar)
	}
}

func isBashCompletion(text string) bool {
	if isZshCompletion(text) || isFishCompletion(text) || isPowerShellCompletion(text) {
		return false
	}
	return strings.Contains(text, "# bash completion") ||
		strings.Contains(text, "complete -C ") ||
		strings.Contains(text, "complete -F ") ||
		strings.Contains(text, "complete -o ")
}

func isZshCompletion(text string) bool {
	return strings.Contains(text, "#compdef") || strings.Contains(text, "compdef ")
}

func isFishCompletion(text string) bool {
	return strings.Contains(text, "complete --command ") || strings.Contains(text, "complete -c ")
}

func isPowerShellCompletion(text string) bool {
	return strings.Contains(text, "Register-ArgumentCompleter")
}

func checkBashSyntax(text string) (err error) {
	bin, err := exec.LookPath("bash")
	if err != nil {
		return nil
	}
	tmp, err := os.CreateTemp("", "arm-completion-*.sh")
	if err != nil {
		return fmt.Errorf("create completion temp file: %w", err)
	}
	name := tmp.Name()
	defer func() {
		if rmErr := os.Remove(name); rmErr != nil && err == nil {
			err = fmt.Errorf("remove completion temp file: %w", rmErr)
		}
	}()
	if _, err = tmp.WriteString(text + "\n"); err != nil {
		if closeErr := tmp.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close completion temp file: %w", closeErr)
		}
		return fmt.Errorf("write completion temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close completion temp file: %w", err)
	}
	cmd := exec.CommandContext(context.Background(), bin, "-n", name) //nolint:gosec // bash from PATH; -n on a temp file we own
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("bash completion-script grammar rejected fixture: %s", msg)
	}
	return nil
}
