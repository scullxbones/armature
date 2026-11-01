package main

import (
	"strings"
	"unicode"
)

// imagesInShell extracts image refs from docker|podman invocations in shell.
func imagesInShell(script string) []string {
	var out []string
	flat := flattenContinuations(script)
	for _, line := range strings.Split(flat, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, imagesInLine(line)...)
	}
	return out
}

func imagesInLine(line string) []string {
	var out []string
	tokens := tokenizeShell(line)
	out = append(out, imagesFromTokens(tokens)...)
	// Recurse into sh/bash -c/-lc quoted scripts.
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t != "sh" && t != "bash" && t != "/bin/sh" && t != "/bin/bash" {
			continue
		}
		j := i + 1
		for j < len(tokens) && strings.HasPrefix(tokens[j], "-") {
			flag := tokens[j]
			j++
			// -c / -lc take a script argument
			if flag == "-c" || flag == "-lc" || strings.HasPrefix(flag, "-c") && len(flag) > 2 {
				if flag == "-c" || flag == "-lc" {
					if j < len(tokens) {
						out = append(out, imagesInShell(tokens[j])...)
					}
				} else {
					// -c'script' glued (rare)
					out = append(out, imagesInShell(strings.TrimPrefix(flag, "-c"))...)
				}
				break
			}
		}
	}
	return out
}

func imagesFromTokens(tokens []string) []string {
	var out []string
	for i := 0; i < len(tokens); i++ {
		cmd := tokens[i]
		// Skip leading FOO=bar env assignments.
		if strings.Contains(cmd, "=") && !strings.HasPrefix(cmd, "-") && !strings.Contains(cmd, "/") && !strings.Contains(cmd, ":") && !strings.Contains(cmd, "@") {
			continue
		}
		if cmd != "docker" && cmd != "podman" {
			continue
		}
		rest := tokens[i+1:]
		img, ok := imageAfterDocker(rest)
		if ok {
			out = append(out, img)
		}
	}
	return out
}

// imageAfterDocker parses [global-flags] [container|image] run|pull|create [flags] IMAGE
func imageAfterDocker(tokens []string) (string, bool) {
	i := 0
	i = skipDockerGlobalFlags(tokens, i)
	if i >= len(tokens) {
		return "", false
	}
	// Optional management command: container | image
	if tokens[i] == "container" || tokens[i] == "image" {
		i++
	}
	if i >= len(tokens) {
		return "", false
	}
	sub := tokens[i]
	if sub != "run" && sub != "pull" && sub != "create" {
		return "", false
	}
	i++
	return imageAfterCommandFlags(sub, tokens[i:])
}

func skipDockerGlobalFlags(tokens []string, i int) int {
	// docker [--config] [--log-level debug] [-D] [-H] ...
	globalWithArg := map[string]bool{
		"--config": true, "--context": true, "--host": true, "-H": true,
		"--log-level": true, "-l": true, "--tls": false, "--tlsverify": false,
	}
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			return i
		}
		if strings.Contains(t, "=") {
			i++
			continue
		}
		if globalWithArg[t] {
			i += 2
			continue
		}
		// boolean global (-D, --debug, …)
		i++
	}
	return i
}

func imageAfterCommandFlags(sub string, tokens []string) (string, bool) {
	// Flags that take a separate value token (not --flag=value).
	withArg := map[string]bool{
		"-v": true, "--volume": true,
		"--mount": true, // value is not an image (false + if treated as image)
		"-w":      true, "--workdir": true,
		"-e": true, "--env": true, "--env-file": true,
		"-u": true, "--user": true,
		"--name": true, "--entrypoint": true,
		"--network": true, "--net": true,
		"--platform": true,
		"-p":         true, "--publish": true,
		"--label": true, // --label k=v (two tokens) or --label=k=v
		"-f":      true, "--file": true,
		"--cidfile": true, "--cgroupns": true, "--cgroup-parent": true,
		"--cpu-shares": true, "--cpus": true, "--memory": true, "-m": true,
		"--restart": true, "--runtime": true, "--security-opt": true,
		"--device": true, "--add-host": true, "--dns": true,
		"--health-cmd": true, "--health-interval": true,
		"--tag": true,
	}
	if sub == "pull" || sub == "create" {
		withArg["-t"] = true // tag
	}
	boolRun := map[string]bool{
		"-i": true, "-d": true, "-P": true,
		"--rm": true, "--tty": true, "--interactive": true, "--detach": true,
		"--init": true, "--privileged": true, "--read-only": true,
		"--quiet": true, "-q": true,
	}
	if sub == "run" {
		boolRun["-t"] = true // TTY
	}
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if !strings.HasPrefix(t, "-") {
			return stripQuotes(t), true
		}
		if strings.Contains(t, "=") {
			i++
			continue
		}
		if boolRun[t] {
			i++
			continue
		}
		if withArg[t] {
			i += 2
			continue
		}
		// Unknown flag: boolean (do not consume next token as image).
		i++
	}
	return "", false
}

func flattenContinuations(s string) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		for strings.HasSuffix(strings.TrimRight(line, " \t"), "\\") {
			line = strings.TrimRight(line, " \t")
			line = strings.TrimSuffix(line, "\\") + " "
			i++
			if i >= len(lines) {
				break
			}
			line += lines[i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// tokenizeShell splits on whitespace while preserving quoted strings as one token
// and respecting simple escapes. Also splits on shell operators outside quotes.
func tokenizeShell(line string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	inSingle, inDouble := false, false
	escaped := false
	for _, r := range line {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if inSingle {
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		if inDouble {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inDouble = false
				continue
			}
			cur.WriteRune(r)
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case ' ', '\t':
			flush()
		case ';', '|', '&', '\n':
			flush()
		default:
			if unicode.IsSpace(r) {
				flush()
			} else {
				cur.WriteRune(r)
			}
		}
	}
	flush()
	return tokens
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
