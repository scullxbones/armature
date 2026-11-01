// workflowimagepins is a static ADVISORY scanner for container-image policy.
// Fail-closed runtime enforcement is CI PATH-shadow
// (scripts/ci-path-shadow-engines.sh) plus scripts/run-container-image.sh.
//
// Holistic pin model (not a docker-CLI tokenizer):
//   - scripts/container-images.manifest lists every allowed name@sha256
//   - scripts/run-container-image.sh is the only allowlisted engine entrypoint
//   - container:/services:/uses: docker:// / Dockerfile FROM must match manifest
//   - any engine token outside the wrapper is reported (advisory on repo scan)
//   - scans workflows (run: + shell:), actions, root action.yml, uses: ./path,
//     Dockerfiles, scripts/** (any depth/ext), and Makefile
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
	// Engine / runtime tokens (PATH-shadowed names + buildx).
	engineToken = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(docker|podman|buildx|nerdctl|ctr|finch|lima|crictl)([^A-Za-z0-9_]|$)`)
	fromLine    = regexp.MustCompile(`(?i)^\s*FROM\s+(.+)$`)
)

const (
	manifestRel = "scripts/container-images.manifest"
	wrapperRel  = "scripts/run-container-image.sh"
)

// Short exempt list for scripts that must mention engine names.
var scriptExempt = map[string]bool{
	"scripts/run-container-image.sh":       true,
	"scripts/ci-path-shadow-engines.sh":    true,
	"scripts/ci-path-shadow-selfcheck.sh":  true,
	"scripts/check-workflow-image-pins.sh": true,
	"scripts/test_check_jobs.sh":           true, // asserts engine-token policy
}

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
	advisory := false
	root := "."
	for len(args) > 0 {
		switch args[0] {
		case "-selftest":
			selftest = true
			args = args[1:]
		case "-advisory":
			advisory = true
			args = args[1:]
		case "-h", "-help", "--help":
			fmt.Fprintf(stderr, "usage: workflowimagepins [-selftest|-advisory] [repo-root]\n")
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
		label := "FAIL"
		if advisory {
			label = "ADVISORY"
		}
		fmt.Fprintf(stderr, "%s: container image policy findings (static; runtime fail-closed is CI PATH-shadow):\n", label)
		for _, f := range findings {
			fmt.Fprintf(stderr, "  %s (%s): %s\n", f.File, f.Path, f.Msg)
		}
		if advisory {
			fmt.Fprintln(stdout, "OK: workflowimagepins advisory scan complete (findings above are non-blocking)")
			return 0
		}
		return 1
	}
	fmt.Fprintln(stdout, "OK: container images are manifest-pinned; no engine tokens outside wrapper")
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
	seenActions := map[string]bool{}

	wfDir := filepath.Join(root, ".github", "workflows")
	if entries, err := os.ReadDir(wfDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(".github", "workflows", name))
			fs, err := checkWorkflowFile(root, filepath.Join(wfDir, name), rel, allowed, seenActions)
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
		if seenActions[rel] {
			return nil
		}
		seenActions[rel] = true
		fs, err := checkActionFile(root, path, rel, allowed, seenActions)
		if err != nil {
			return err
		}
		out = append(out, fs...)
		return nil
	}); err != nil {
		return nil, err
	}

	for _, name := range []string{"action.yml", "action.yaml"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			if !seenActions[name] {
				seenActions[name] = true
				fs, err := checkActionFile(root, p, name, allowed, seenActions)
				if err != nil {
					return nil, err
				}
				out = append(out, fs...)
			}
		}
	}

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

	// scripts/** any depth / any extension; short exempt list only.
	scriptsDir := filepath.Join(root, "scripts")
	if err := filepath.WalkDir(scriptsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if scriptExempt[rel] || sameFile(path, wrapperAbs) || strings.HasPrefix(rel, "scripts/workflowimagepins/") {
			return nil
		}
		out = append(out, tokenFindingsInFile(path, rel)...)
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	makefile := filepath.Join(root, "Makefile")
	if _, err := os.Stat(makefile); err == nil {
		out = append(out, tokenFindingsInFile(makefile, "Makefile")...)
	}

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

func checkWorkflowFile(root, absPath, rel string, allowed map[string]bool, seenActions map[string]bool) ([]finding, error) {
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
		defaultsShell := ""
		if defaults, ok := job["defaults"].(map[string]any); ok {
			if runMap, ok := defaults["run"].(map[string]any); ok {
				if s, ok := runMap["shell"].(string); ok {
					defaultsShell = s
				}
			}
		}
		if defaultsShell != "" {
			out = append(out, tokenFindings(defaultsShell, rel, base+".defaults.run.shell")...)
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
					fs, err := checkLocalActionUse(root, rel, spath+".uses", uses, allowed, seenActions)
					if err != nil {
						return nil, err
					}
					out = append(out, fs...)
				}
			}
			if shell, ok := step["shell"].(string); ok {
				out = append(out, tokenFindings(shell, rel, spath+".shell")...)
			}
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

func checkLocalActionUse(root, fromFile, path, uses string, allowed map[string]bool, seenActions map[string]bool) ([]finding, error) {
	actionDir := filepath.Clean(filepath.Join(root, filepath.FromSlash(uses)))
	for _, name := range []string{"action.yml", "action.yaml"} {
		p := filepath.Join(actionDir, name)
		if _, err := os.Stat(p); err == nil {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			rel = filepath.ToSlash(rel)
			if seenActions[rel] {
				return nil, nil
			}
			seenActions[rel] = true
			return checkActionFile(root, p, rel, allowed, seenActions)
		}
	}
	return []finding{{
		File: fromFile,
		Path: path,
		Msg:  fmt.Sprintf("local action %q has no action.yml", uses),
	}}, nil
}

func checkActionFile(root, absPath, rel string, allowed map[string]bool, seenActions map[string]bool) ([]finding, error) {
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
				switch {
				case strings.HasPrefix(uses, "docker://"):
					out = append(out, refFinding(rel, spath+".uses", strings.TrimPrefix(uses, "docker://"), allowed)...)
				case strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../"):
					fs, err := checkLocalActionUse(root, rel, spath+".uses", uses, allowed, seenActions)
					if err != nil {
						return nil, err
					}
					out = append(out, fs...)
				}
			}
			if shell, ok := step["shell"].(string); ok {
				out = append(out, tokenFindings(shell, rel, spath+".shell")...)
			}
			if runScript, ok := stepRun(step); ok {
				out = append(out, tokenFindings(runScript, rel, spath+".run")...)
			}
		}
	}
	_ = root
	return out, nil
}

func checkDockerfile(absPath, rel string, allowed map[string]bool) ([]finding, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var out []finding
	stages := map[string]bool{}
	for i, line := range strings.Split(string(data), "\n") {
		m := fromLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := strings.TrimSpace(m[1])
		// FROM [--platform=...] <image|stage> [AS name]
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		idx := 0
		switch {
		case strings.HasPrefix(strings.ToLower(fields[0]), "--platform="):
			idx = 1
		case strings.EqualFold(fields[0], "--platform"):
			if len(fields) < 3 {
				out = append(out, finding{File: rel, Path: fmt.Sprintf("FROM:%d", i+1), Msg: "malformed FROM --platform"})
				continue
			}
			idx = 2
		}
		if idx >= len(fields) {
			out = append(out, finding{File: rel, Path: fmt.Sprintf("FROM:%d", i+1), Msg: "FROM missing image"})
			continue
		}
		ref := fields[idx]
		stageName := ""
		for j := idx + 1; j < len(fields); j++ {
			if strings.EqualFold(fields[j], "AS") && j+1 < len(fields) {
				stageName = fields[j+1]
				break
			}
		}
		if stageName != "" {
			stages[stageName] = true
		}
		// scratch and prior stage names need no digest pin.
		if strings.EqualFold(ref, "scratch") || stages[ref] {
			continue
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
	var out []finding
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !engineToken.MatchString(line) {
			continue
		}
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if len(trim) > 120 {
			trim = trim[:117] + "..."
		}
		out = append(out, finding{
			File: file,
			Path: fmt.Sprintf("%s:%d", path, i+1),
			Msg:  fmt.Sprintf("engine token outside %s: %s", wrapperRel, trim),
		})
	}
	return out
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
