// workflowimagepins fails closed when workflows/actions/scripts reach
// docker|podman|buildx outside the sole allowlisted wrapper, or when
// declarative container images are not exact values from the image manifest.
//
// Holistic pin model (not a docker-CLI tokenizer):
//   - scripts/container-images.manifest lists every allowed name@sha256
//   - scripts/run-container-image.sh is the only allowed docker entrypoint
//   - container:/services:/uses: docker:// / Dockerfile FROM must match manifest
//   - any docker|podman|buildx token outside the wrapper fails
//   - scans .github/workflows, .github/actions, root action.yml, uses: ./path,
//     Dockerfiles, scripts/*.sh, and Makefile
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	digestPin = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)
	// Word-boundary engine tokens (also catches compose/buildx via "docker").
	engineToken = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(docker|podman|buildx)([^A-Za-z0-9_]|$)`)
	fromLine    = regexp.MustCompile(`(?i)^\s*FROM\s+(\S+)`)
)

const (
	manifestRel = "scripts/container-images.manifest"
	wrapperRel  = "scripts/run-container-image.sh"
)

type finding struct {
	File string
	Path string
	Msg  string
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
	findings, err := checkRepo(abs)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1
	}
	if len(findings) > 0 {
		fmt.Fprintln(stderr, "FAIL: container image policy violations:")
		for _, f := range findings {
			fmt.Fprintf(stderr, "  %s (%s): %s\n", f.File, f.Path, f.Msg)
		}
		return 1
	}
	fmt.Fprintln(stdout, "OK: container images are manifest-pinned; no raw docker|podman|buildx outside wrapper")
	return 0
}

func checkRepo(root string) ([]finding, error) {
	manPath := filepath.Join(root, filepath.FromSlash(manifestRel))
	allowed, err := loadManifest(manPath)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("%s: empty manifest", manifestRel)
	}
	return scanTree(root, allowed)
}

func loadManifest(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s:%d: want key and name@sha256", path, i+1)
		}
		ref := fields[len(fields)-1]
		if !digestPin.MatchString(ref) {
			return nil, fmt.Errorf("%s:%d: value must be name@sha256:<64-hex>, got %q", path, i+1, ref)
		}
		out[ref] = true
	}
	return out, nil
}

func scanTree(root string, allowed map[string]bool) ([]finding, error) {
	var out []finding
	wrapperAbs := filepath.Join(root, filepath.FromSlash(wrapperRel))

	// Declarative + token scan of workflows/actions.
	wfDir := filepath.Join(root, ".github", "workflows")
	if entries, err := os.ReadDir(wfDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(".github", "workflows", name))
			fs, err := checkWorkflowFile(root, filepath.Join(wfDir, name), rel, allowed)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	actionsRoot := filepath.Join(root, ".github", "actions")
	if err := filepath.WalkDir(actionsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := strings.ToLower(d.Name())
		if base != "action.yml" && base != "action.yaml" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		fs, err := checkActionFile(path, rel, allowed)
		if err != nil {
			return err
		}
		out = append(out, fs...)
		return nil
	}); err != nil {
		return nil, err
	}

	// Root action.yml (composite/docker action at repo root).
	for _, name := range []string{"action.yml", "action.yaml"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			fs, err := checkActionFile(p, name, allowed)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
		}
	}

	// Dockerfiles anywhere except testdata / vendor-ish.
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "testdata" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		if base != "Dockerfile" && !strings.HasPrefix(base, "Dockerfile.") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		fs, err := checkDockerfile(path, rel, allowed)
		if err != nil {
			return err
		}
		out = append(out, fs...)
		return nil
	}); err != nil {
		return nil, err
	}

	// scripts/*.sh + Makefile: no engine tokens outside wrapper.
	scriptsDir := filepath.Join(root, "scripts")
	if entries, err := os.ReadDir(scriptsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".sh") {
				continue
			}
			// Harness/test scripts may mention engine tokens in assertions.
			if strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.sh") {
				continue
			}
			abs := filepath.Join(scriptsDir, name)
			if sameFile(abs, wrapperAbs) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join("scripts", name))
			out = append(out, tokenFindingsInFile(abs, rel)...)
		}
	}
	makefile := filepath.Join(root, "Makefile")
	if _, err := os.Stat(makefile); err == nil {
		out = append(out, tokenFindingsInFile(makefile, "Makefile")...)
	}

	_ = wrapperAbs
	return out, nil
}

func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return os.SameFile(ai, bi)
}

func checkWorkflowFile(root, absPath, rel string, allowed map[string]bool) ([]finding, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var out []finding
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: yaml parse: %w", rel, err)
	}
	jobs, ok := doc["jobs"].(map[string]any)
	if !ok {
		return out, nil
	}
	for jobName, rawJob := range jobs {
		job, ok := rawJob.(map[string]any)
		if !ok {
			continue
		}
		base := "jobs." + jobName
		if c, ok := job["container"]; ok {
			out = append(out, declarativeImageFindings(rel, base+".container", c, allowed)...)
		}
		if servicesRaw, ok := job["services"].(map[string]any); ok {
			for svcName, rawSvc := range servicesRaw {
				svc, ok := rawSvc.(map[string]any)
				if !ok {
					continue
				}
				if img, ok := svc["image"]; ok {
					out = append(out, declarativeImageFindings(rel, base+".services."+svcName+".image", img, allowed)...)
				}
			}
		}
		for i, step := range jobSteps(job) {
			spath := fmt.Sprintf("%s.steps[%d]", base, i)
			if uses, ok := step["uses"].(string); ok {
				uses = strings.TrimSpace(uses)
				switch {
				case strings.HasPrefix(uses, "docker://"):
					ref := strings.TrimPrefix(uses, "docker://")
					out = append(out, refFinding(rel, spath+".uses", ref, allowed)...)
				case strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../"):
					fs, err := checkLocalActionUse(root, rel, spath+".uses", uses, allowed)
					if err != nil {
						return nil, err
					}
					out = append(out, fs...)
				}
			}
			// Engine tokens only in run: bodies (not in uses:/using: declarative keys).
			if runScript, ok := stepRun(step); ok {
				out = append(out, tokenFindings(runScript, rel, spath+".run")...)
			}
		}
	}
	return out, nil
}

func stepRun(step map[string]any) (string, bool) {
	raw, ok := step["run"]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	return s, ok
}

func checkLocalActionUse(root, fromFile, path, uses string, allowed map[string]bool) ([]finding, error) {
	// GitHub Actions resolves uses: ./path relative to the repository root.
	actionDir := filepath.Clean(filepath.Join(root, filepath.FromSlash(uses)))
	for _, name := range []string{"action.yml", "action.yaml"} {
		p := filepath.Join(actionDir, name)
		if _, err := os.Stat(p); err == nil {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			return checkActionFile(p, filepath.ToSlash(rel), allowed)
		}
	}
	return []finding{{
		File: fromFile,
		Path: path,
		Msg:  fmt.Sprintf("local action %q has no action.yml", uses),
	}}, nil
}

func checkActionFile(absPath, rel string, allowed map[string]bool) ([]finding, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var out []finding
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: yaml parse: %w", rel, err)
	}
	if runs, ok := doc["runs"].(map[string]any); ok {
		if using, ok := runs["using"].(string); ok && using == "docker" {
			if img, ok := runs["image"]; ok {
				out = append(out, declarativeImageFindings(rel, "runs.image", img, allowed)...)
			}
		}
		for i, step := range jobSteps(runs) {
			spath := fmt.Sprintf("runs.steps[%d]", i)
			if uses, ok := step["uses"].(string); ok {
				uses = strings.TrimSpace(uses)
				if strings.HasPrefix(uses, "docker://") {
					out = append(out, refFinding(rel, spath+".uses", strings.TrimPrefix(uses, "docker://"), allowed)...)
				}
			}
			if runScript, ok := stepRun(step); ok {
				out = append(out, tokenFindings(runScript, rel, spath+".run")...)
			}
		}
	}
	return out, nil
}

func checkDockerfile(absPath, rel string, allowed map[string]bool) ([]finding, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var out []finding
	for i, line := range strings.Split(string(data), "\n") {
		m := fromLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ref := strings.TrimSpace(m[1])
		// Strip stage AS name if present already handled — FROM img AS x → first token.
		if idx := strings.IndexAny(ref, " \t"); idx >= 0 {
			ref = ref[:idx]
		}
		out = append(out, refFinding(rel, fmt.Sprintf("FROM:%d", i+1), ref, allowed)...)
	}
	return out, nil
}

func declarativeImageFindings(file, path string, raw any, allowed map[string]bool) []finding {
	switch v := raw.(type) {
	case string:
		return refFinding(file, path, strings.TrimSpace(v), allowed)
	case map[string]any:
		if img, ok := v["image"]; ok {
			if s, ok := img.(string); ok {
				return refFinding(file, path+".image", strings.TrimSpace(s), allowed)
			}
		}
	}
	return nil
}

func refFinding(file, path, ref string, allowed map[string]bool) []finding {
	if ref == "" {
		return []finding{{File: file, Path: path, Msg: "empty image ref (unpinned)"}}
	}
	if !digestPin.MatchString(ref) {
		return []finding{{File: file, Path: path, Msg: fmt.Sprintf("not name@sha256: %q", ref)}}
	}
	if !allowed[ref] {
		return []finding{{File: file, Path: path, Msg: fmt.Sprintf("digest not in %s: %q", manifestRel, ref)}}
	}
	return nil
}

func tokenFindingsInFile(abs, rel string) []finding {
	data, err := os.ReadFile(abs)
	if err != nil {
		return []finding{{File: rel, Path: "file", Msg: err.Error()}}
	}
	return tokenFindings(string(data), rel, "file")
}

func tokenFindings(content, file, path string) []finding {
	if !engineToken.MatchString(content) {
		return nil
	}
	// Report once per file with a short excerpt of the first hit.
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if engineToken.MatchString(line) {
			trim := strings.TrimSpace(line)
			if len(trim) > 120 {
				trim = trim[:117] + "..."
			}
			return []finding{{
				File: file,
				Path: fmt.Sprintf("%s:%d", path, i+1),
				Msg:  fmt.Sprintf("docker|podman|buildx token outside %s: %s", wrapperRel, trim),
			}}
		}
	}
	return []finding{{
		File: file,
		Path: path,
		Msg:  fmt.Sprintf("docker|podman|buildx token outside %s", wrapperRel),
	}}
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
