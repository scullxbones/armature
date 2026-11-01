// workflowimagepins fails closed when GitHub workflow files reference
// container images without an @sha256:<64-hex> digest pin.
//
// Images collected via YAML parse (gopkg.in/yaml.v3), not line grep:
//   - jobs.*.container (string or mapping .image)
//   - jobs.*.services.*.image
//   - steps[].uses starting with docker://
//   - image args to docker|podman run|pull|create inside steps[].run
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	digestPin  = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	bareDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type imageRef struct {
	File string
	Path string // YAML path / locator
	Ref  string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	selftest := false
	root := "."
	for len(args) > 0 {
		switch args[0] {
		case "-selftest":
			selftest = true
			args = args[1:]
		case "-h", "-help", "--help":
			fmt.Fprintf(stderr, "usage: workflowimagepins [-selftest] [repo-root]\n")
			return 0
		default:
			if strings.HasPrefix(args[0], "-") {
				fmt.Fprintf(stderr, "FAIL: unknown flag %s\n", args[0])
				return 2
			}
			root = args[0]
			args = args[1:]
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1
	}
	if selftest {
		if err := runSelftest(abs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "FAIL: selftest: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "OK: workflow image-pin selftest (unpinned fail, pinned pass)")
		return 0
	}
	refs, err := collectRepo(abs)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1
	}
	var bad []imageRef
	for _, r := range refs {
		if !pinned(r.Ref) {
			bad = append(bad, r)
		}
	}
	if len(bad) > 0 {
		fmt.Fprintln(stderr, "FAIL: workflow images must be digest-pinned (@sha256:…):")
		for _, r := range bad {
			fmt.Fprintf(stderr, "  %s (%s): %q\n", r.File, r.Path, r.Ref)
		}
		return 1
	}
	fmt.Fprintln(stdout, "OK: workflow docker/container images are @sha256-pinned")
	return 0
}

func pinned(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return true
	}
	if bareDigest.MatchString(ref) {
		return true
	}
	return digestPin.MatchString(ref)
}

func collectRepo(root string) ([]imageRef, error) {
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []imageRef
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".github", "workflows", name))
		refs, err := collectFile(filepath.Join(dir, name), rel)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, refs...)
	}
	return out, nil
}

func collectFile(absPath, rel string) ([]imageRef, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return collectDoc(rel, doc), nil
}

func collectDoc(rel string, doc map[string]any) []imageRef {
	var out []imageRef
	jobsRaw, ok := doc["jobs"]
	if !ok {
		return nil
	}
	jobs, ok := jobsRaw.(map[string]any)
	if !ok {
		return nil
	}
	for jobName, rawJob := range jobs {
		job, ok := rawJob.(map[string]any)
		if !ok {
			continue
		}
		base := "jobs." + jobName

		if c, ok := job["container"]; ok {
			out = append(out, containerRefs(rel, base+".container", c)...)
		}
		if servicesRaw, ok := job["services"]; ok {
			services, ok := servicesRaw.(map[string]any)
			if !ok {
				continue
			}
			for svcName, rawSvc := range services {
				svc, ok := rawSvc.(map[string]any)
				if !ok {
					continue
				}
				if img, ok := svc["image"].(string); ok && strings.TrimSpace(img) != "" {
					out = append(out, imageRef{
						File: rel,
						Path: base + ".services." + svcName + ".image",
						Ref:  img,
					})
				}
			}
		}
		steps := jobSteps(job)
		for i, step := range steps {
			spath := fmt.Sprintf("%s.steps[%d]", base, i)
			if uses, ok := step["uses"].(string); ok {
				uses = strings.TrimSpace(uses)
				if strings.HasPrefix(uses, "docker://") {
					out = append(out, imageRef{
						File: rel,
						Path: spath + ".uses",
						Ref:  strings.TrimPrefix(uses, "docker://"),
					})
				}
			}
			if runScript, ok := stepRun(step); ok {
				for _, img := range imagesInShell(runScript) {
					out = append(out, imageRef{
						File: rel,
						Path: spath + ".run",
						Ref:  img,
					})
				}
			}
		}
	}
	return out
}

func containerRefs(rel, path string, c any) []imageRef {
	switch v := c.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []imageRef{{File: rel, Path: path, Ref: v}}
	case map[string]any:
		if img, ok := v["image"].(string); ok && strings.TrimSpace(img) != "" {
			return []imageRef{{File: rel, Path: path + ".image", Ref: img}}
		}
	}
	return nil
}

func jobSteps(job map[string]any) []map[string]any {
	raw, ok := job["steps"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stepRun(step map[string]any) (string, bool) {
	raw, ok := step["run"]
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

// imagesInShell extracts image refs from docker|podman run|pull|create invocations.
func imagesInShell(script string) []string {
	var out []string
	// Flatten line continuations so flags/image on following lines are visible.
	flat := flattenContinuations(script)
	for _, line := range strings.Split(flat, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Allow leading env assignments: FOO=bar docker run ...
		tokens := shellTokens(line)
		for i := 0; i < len(tokens); i++ {
			cmd := tokens[i]
			if cmd != "docker" && cmd != "podman" {
				continue
			}
			if i+1 >= len(tokens) {
				continue
			}
			sub := tokens[i+1]
			if sub != "run" && sub != "pull" && sub != "create" {
				continue
			}
			if img, ok := imageAfterDockerFlags(tokens[i+2:]); ok {
				out = append(out, img)
			}
		}
	}
	return out
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

func shellTokens(line string) []string {
	// Strip shell operators to avoid treating them as images; keep simple split.
	line = strings.ReplaceAll(line, "&&", " ")
	line = strings.ReplaceAll(line, "||", " ")
	line = strings.ReplaceAll(line, ";", " ")
	line = strings.ReplaceAll(line, "|", " ")
	fields := strings.Fields(line)
	var out []string
	for _, f := range fields {
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "-") && !strings.Contains(f, "://") && !strings.Contains(f, "/") {
			// FOO=bar — skip assignment token
			if !strings.Contains(f, ":") {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func imageAfterDockerFlags(tokens []string) (string, bool) {
	flagsWithArg := map[string]bool{
		"-v": true, "--volume": true,
		"-w": true, "--workdir": true,
		"-e": true, "--env": true,
		"-u": true, "--user": true,
		"--name": true, "--entrypoint": true,
		"--network": true, "--platform": true,
		"-p": true, "--publish": true,
		"-t":    false, // -t often alone; --tag takes arg for some commands
		"--tag": true,
		"-f":    true, "--file": true,
	}
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if strings.HasPrefix(t, "-") {
			if strings.Contains(t, "=") {
				i++
				continue
			}
			if flagsWithArg[t] {
				i += 2
				continue
			}
			// boolean / unknown short flags
			i++
			continue
		}
		return t, true
	}
	return "", false
}
