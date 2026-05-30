package orchestrate

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	AuthModeAuto         = "auto"
	AuthModeInheritEnv   = "inherit-env"
	AuthModeEnvFile      = "env-file"
	AuthModeOAuthSession = "oauth-session"
)

type AuthConfig struct {
	Mode    string `json:"mode,omitempty"`
	EnvFile string `json:"env_file,omitempty"`
}

type AuthPlan struct {
	Harness         string
	Provider        string
	Mode            string
	Source          string
	APIKeyDetected  bool
	SessionDetected bool
	Env             map[string]string
	EndpointHint    string
}

var authStatusCommand = runAuthStatusCommand
var lookPathCommand = exec.LookPath

func ResolveAuthPlan(harness string, cfg AuthConfig) (AuthPlan, error) {
	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = AuthModeAuto
	}

	plan := AuthPlan{
		Harness:      harness,
		Provider:     providerForHarness(harness),
		Mode:         mode,
		Env:          map[string]string{},
		EndpointHint: endpointHintForHarness(harness),
	}

	envVals := map[string]string{}
	if mode == AuthModeEnvFile || mode == AuthModeAuto {
		vals, err := loadEnvFile(cfg.EnvFile)
		if err != nil {
			return plan, err
		}
		for k, v := range vals {
			envVals[k] = v
			plan.Env[k] = v
		}
	}

	apiVars := apiKeyVarsForHarness(harness)
	for _, name := range apiVars {
		if envVals[name] != "" || os.Getenv(name) != "" {
			plan.APIKeyDetected = true
			break
		}
	}
	if mode == AuthModeInheritEnv && !plan.APIKeyDetected {
		return plan, fmt.Errorf("missing API key env for %s (expected one of: %s)", harness, strings.Join(apiVars, ", "))
	}

	if plan.APIKeyDetected {
		plan.Source = "api-key"
		return plan, nil
	}

	if _, err := harnessBinaryPath(harness); err != nil {
		return plan, err
	}

	if mode == AuthModeEnvFile {
		return plan, fmt.Errorf("missing API key env for %s in env-file mode (expected one of: %s)", harness, strings.Join(apiVars, ", "))
	}

	ok, err := authStatusCommand(harness)
	if err != nil {
		if harness == "devin" && (mode == AuthModeAuto || mode == AuthModeOAuthSession) {
			plan.Source = "oauth-session"
			return plan, nil
		}
		if mode == AuthModeOAuthSession {
			return plan, err
		}
		// auto mode: keep going to final failure with remediation text below.
	}
	plan.SessionDetected = ok
	if ok {
		plan.Source = "oauth-session"
		return plan, nil
	}

	return plan, fmt.Errorf("%s auth unavailable: set API key env (%s) or login via %s",
		harness, strings.Join(apiVars, ", "), loginHintForHarness(harness))
}

func providerForHarness(harness string) string {
	switch harness {
	case "codex":
		return "openai"
	case "claude":
		return "anthropic"
	case "devin":
		return "devin"
	default:
		return "unknown"
	}
}

func apiKeyVarsForHarness(harness string) []string {
	switch harness {
	case "codex":
		return []string{"OPENAI_API_KEY", "CODEX_ACCESS_TOKEN"}
	case "claude":
		return []string{"ANTHROPIC_API_KEY"}
	case "devin":
		return []string{"DEVIN_API_KEY"}
	default:
		return []string{"API_KEY"}
	}
}

func loginHintForHarness(harness string) string {
	switch harness {
	case "codex":
		return "codex login"
	case "claude":
		return "claude auth login"
	case "devin":
		return "devin login"
	default:
		return "login with your harness CLI"
	}
}

func endpointHintForHarness(harness string) string {
	switch harness {
	case "codex":
		return "api.openai.com (responses API/websocket)"
	case "claude":
		return "api.anthropic.com (Claude API)"
	case "devin":
		return "Devin service API (provider-managed endpoint)"
	default:
		return "unknown"
	}
}

func harnessBinaryPath(harness string) (string, error) {
	bin := harness
	switch harness {
	case "codex", "claude", "devin":
	default:
		return "", fmt.Errorf("unknown harness %q", harness)
	}
	path, err := lookPathCommand(bin)
	if err != nil {
		return "", fmt.Errorf("%s CLI not found on PATH; install %s before orchestrating", harness, harness)
	}
	return path, nil
}

func runAuthStatusCommand(harness string) (bool, error) {
	var cmd *exec.Cmd
	switch harness {
	case "codex":
		cmd = exec.Command("codex", "login", "status")
	case "claude":
		cmd = exec.Command("claude", "auth", "status")
	default:
		return false, fmt.Errorf("oauth-session not supported for harness %q", harness)
	}
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func loadEnvFile(path string) (map[string]string, error) {
	vals := map[string]string{}
	if strings.TrimSpace(path) == "" {
		return vals, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.Trim(strings.TrimSpace(line[i+1:]), `"'`)
		vals[k] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	return vals, nil
}
