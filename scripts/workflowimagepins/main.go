// workflowimagepins scans container-image policy and GitHub Action pins.
// Fail-closed runtime enforcement for images is CI PATH-shadow
// (scripts/ci-path-shadow-engines.sh) plus scripts/run-container-image.sh.
//
// Holistic pin model (not a docker-CLI tokenizer):
//   - scripts/container-images.manifest lists every allowed name@sha256
//   - scripts/run-container-image.sh is the only allowlisted engine entrypoint
//   - container:/services:/uses: docker:// / Dockerfile FROM must match manifest
//   - ref/digest findings are BLOCKING on repo scan
//   - external uses: (owner/repo[/path]@ref, including first-party actions and
//     job-level reusable workflows) must be a 40-character lowercase commit SHA
//   - repository-local uses (./, ../, $/) are exempt only when the resolved
//     path stays inside the checkout; docker:// and image: are not actions
//   - engine-word tokens outside the wrapper are ADVISORY only on repo scan
//   - scans workflows (run: + shell: + uses:), actions, root action.yml,
//     Dockerfiles, scripts/** (any depth/ext), and Makefile
//
// YAML parse errors fail the scan. .git, testdata, vendor, and node_modules
// are not production roots; fixtures under those trees are not findings.
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
	// GitHub Action pins are commit SHAs from the GitHub API: 40 lowercase hex.
	actionSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hexRef    = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

const (
	manifestRel = "scripts/container-images.manifest"
	wrapperRel  = "scripts/run-container-image.sh"

	kindEngine = "engine"
	kindRef    = "ref"
	kindAction = "action"
)

// scriptExempt returns the short exempt list for scripts that must mention
// engine names (function, not a package global — gochecknoglobals).
func scriptExempt() map[string]bool {
	return map[string]bool{
		"scripts/run-container-image.sh":       true,
		"scripts/ci-path-shadow-engines.sh":    true,
		"scripts/ci-path-shadow-selfcheck.sh":  true,
		"scripts/check-workflow-image-pins.sh": true,
		"scripts/check-no-engine-bin-env.sh":   true,
		"scripts/test_check_jobs.sh":           true, // asserts engine-token policy
	}
}

type finding struct {
	File string
	Path string
	Msg  string
	Kind string // kindEngine (advisory) or kindRef (blocking)
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
		case "-advisory":
			// Legacy no-op: engine words are always advisory; refs always block.
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
		fmt.Fprintln(stdout, "OK: workflow image-pin and action-pin selftest (unpinned fail, pinned pass)")
		return 0
	}
	findings, err := checkRepo(abs)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1
	}
	var engines, refs, actions []finding
	for _, f := range findings {
		switch f.Kind {
		case kindEngine:
			engines = append(engines, f)
		case kindAction:
			actions = append(actions, f)
		default:
			refs = append(refs, f)
		}
	}
	if len(engines) > 0 {
		fmt.Fprintf(stderr, "ADVISORY: engine-word findings (non-blocking; runtime fail-closed is CI PATH-shadow):\n")
		for _, f := range engines {
			fmt.Fprintf(stderr, "  %s (%s): %s\n", f.File, f.Path, f.Msg)
		}
	}
	if len(refs) > 0 {
		fmt.Fprintf(stderr, "FAIL: unpinned/unknown container image refs (blocking):\n")
		for _, f := range refs {
			fmt.Fprintf(stderr, "  %s (%s): %s\n", f.File, f.Path, f.Msg)
		}
	}
	if len(actions) > 0 {
		fmt.Fprintf(stderr, "FAIL: unpinned GitHub Action uses (blocking):\n")
		for _, f := range actions {
			fmt.Fprintf(stderr, "  %s (%s): %s\n", f.File, f.Path, f.Msg)
		}
	}
	if len(refs) > 0 || len(actions) > 0 {
		return 1
	}
	if len(engines) > 0 {
		fmt.Fprintln(stdout, "OK: container image refs manifest-pinned; external actions SHA-pinned (engine-word findings advisory only)")
		return 0
	}
	fmt.Fprintln(stdout, "OK: container images are manifest-pinned; external actions are SHA-pinned; no engine tokens outside wrapper")
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
		if scriptExempt()[rel] || sameFile(path, wrapperAbs) || strings.HasPrefix(rel, "scripts/workflowimagepins/") {
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
	if seenActions[rel] {
		return nil, nil
	}
	seenActions[rel] = true
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var out []finding
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: yaml parse: %w", rel, err)
	}
	rawJobs, ok := doc["jobs"]
	if !ok {
		return out, nil
	}
	jobs, ok := rawJobs.(map[string]any)
	if !ok {
		return []finding{{
			File: rel,
			Path: "jobs",
			Msg:  "jobs is not a mapping",
			Kind: kindAction,
		}}, nil
	}
	for jobName, rawJob := range jobs {
		job, ok := rawJob.(map[string]any)
		if !ok {
			out = append(out, finding{
				File: rel,
				Path: "jobs." + jobName,
				Msg:  "job is not a mapping",
				Kind: kindAction,
			})
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
		if rawUses, ok := job["uses"]; ok {
			fs, err := consumeUsesValue(root, rel, base+".uses", rawUses, allowed, seenActions)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
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
		fs, err := scanSteps(root, rel, base, job, allowed, seenActions)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
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
	lexical, inside := resolveLocal(root, uses)
	if !inside {
		return []finding{{
			File: fromFile,
			Path: path,
			Msg:  fmt.Sprintf("local reference %q escapes the checkout", uses),
			Kind: kindAction,
		}}, nil
	}
	resolved, err := filepath.EvalSymlinks(lexical)
	if err != nil {
		if os.IsNotExist(err) {
			return []finding{{
				File: fromFile,
				Path: path,
				Msg:  missingLocalMsg(uses),
				Kind: kindAction,
			}}, nil
		}
		return nil, err
	}
	if !staysInside(root, resolved) {
		return []finding{{
			File: fromFile,
			Path: path,
			Msg:  fmt.Sprintf("local reference %q escapes the checkout", uses),
			Kind: kindAction,
		}}, nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		low := strings.ToLower(info.Name())
		if strings.HasSuffix(low, ".yml") || strings.HasSuffix(low, ".yaml") {
			rel, err := relUnderRoot(root, resolved)
			if err != nil {
				return nil, err
			}
			return checkWorkflowFile(root, resolved, rel, allowed, seenActions)
		}
		return []finding{{
			File: fromFile,
			Path: path,
			Msg:  fmt.Sprintf("local reference %q is not an action directory or workflow file", uses),
			Kind: kindAction,
		}}, nil
	}
	for _, name := range []string{"action.yml", "action.yaml"} {
		p := filepath.Join(resolved, name)
		st, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if st.IsDir() {
			continue
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil, err
		}
		if !staysInside(root, target) {
			return []finding{{
				File: fromFile,
				Path: path,
				Msg:  fmt.Sprintf("local reference %q escapes the checkout", uses),
				Kind: kindAction,
			}}, nil
		}
		rel, err := relUnderRoot(root, target)
		if err != nil {
			return nil, err
		}
		if seenActions[rel] {
			return nil, nil
		}
		seenActions[rel] = true
		return checkActionFile(root, target, rel, allowed, seenActions)
	}
	return []finding{{
		File: fromFile,
		Path: path,
		Msg:  missingLocalMsg(uses),
		Kind: kindAction,
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
	if rawRuns, ok := doc["runs"]; ok {
		runs, ok := rawRuns.(map[string]any)
		if !ok {
			return []finding{{
				File: rel,
				Path: "runs",
				Msg:  "runs is not a mapping",
				Kind: kindAction,
			}}, nil
		}
		if using, ok := runs["using"].(string); ok && using == "docker" {
			if img, ok := runs["image"]; ok {
				out = append(out, declarativeImageFindings(rel, "runs.image", img, allowed)...)
			}
		}
		fs, err := scanSteps(root, rel, "runs", runs, allowed, seenActions)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
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
				out = append(out, finding{File: rel, Path: fmt.Sprintf("FROM:%d", i+1), Msg: "malformed FROM --platform", Kind: kindRef})
				continue
			}
			idx = 2
		}
		if idx >= len(fields) {
			out = append(out, finding{File: rel, Path: fmt.Sprintf("FROM:%d", i+1), Msg: "FROM missing image", Kind: kindRef})
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
		return []finding{{File: file, Path: path, Msg: "empty image ref (unpinned)", Kind: kindRef}}
	}
	if !digestPin.MatchString(ref) {
		return []finding{{File: file, Path: path, Msg: fmt.Sprintf("not name@sha256: %q", ref), Kind: kindRef}}
	}
	if !allowed[ref] {
		return []finding{{File: file, Path: path, Msg: fmt.Sprintf("digest not in %s: %q", manifestRel, ref), Kind: kindRef}}
	}
	return nil
}

func tokenFindingsInFile(abs, rel string) []finding {
	data, err := os.ReadFile(abs)
	if err != nil {
		return []finding{{File: rel, Path: "file", Msg: err.Error(), Kind: kindRef}}
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
			Kind: kindEngine,
		})
	}
	return out
}

func scanSteps(root, rel, base string, parent map[string]any, allowed map[string]bool, seen map[string]bool) ([]finding, error) {
	raw, ok := parent["steps"]
	if !ok {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return []finding{{
			File: rel,
			Path: base + ".steps",
			Msg:  "steps is not a list",
			Kind: kindAction,
		}}, nil
	}
	var out []finding
	for i, item := range list {
		spath := fmt.Sprintf("%s.steps[%d]", base, i)
		step, ok := item.(map[string]any)
		if !ok {
			out = append(out, finding{
				File: rel,
				Path: spath,
				Msg:  "step is not a mapping",
				Kind: kindAction,
			})
			continue
		}
		if rawUses, exists := step["uses"]; exists {
			fs, err := consumeUsesValue(root, rel, spath+".uses", rawUses, allowed, seen)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
		}
		if shell, ok := step["shell"].(string); ok {
			out = append(out, tokenFindings(shell, rel, spath+".shell")...)
		}
		if runScript, ok := stepRun(step); ok {
			out = append(out, tokenFindings(runScript, rel, spath+".run")...)
		}
	}
	return out, nil
}

func consumeUsesValue(root, fromFile, path string, raw any, allowed map[string]bool, seen map[string]bool) ([]finding, error) {
	uses, ok := raw.(string)
	if !ok {
		return []finding{{
			File: fromFile,
			Path: path,
			Msg:  "uses is not a string",
			Kind: kindAction,
		}}, nil
	}
	return consumeUses(root, fromFile, path, uses, allowed, seen)
}

func consumeUses(root, fromFile, path, uses string, allowed map[string]bool, seen map[string]bool) ([]finding, error) {
	uses = strings.TrimSpace(uses)
	switch {
	case uses == "":
		return []finding{{
			File: fromFile,
			Path: path,
			Msg:  "empty uses",
			Kind: kindAction,
		}}, nil
	case strings.HasPrefix(uses, "docker://"):
		return refFinding(fromFile, path, strings.TrimPrefix(uses, "docker://"), allowed), nil
	case isLocalUse(uses):
		return checkLocalActionUse(root, fromFile, path, uses, allowed, seen)
	default:
		if msg := externalPinIssue(uses); msg != "" {
			return []finding{{
				File: fromFile,
				Path: path,
				Msg:  msg,
				Kind: kindAction,
			}}, nil
		}
		return nil, nil
	}
}

func isLocalUse(uses string) bool {
	switch {
	case uses == ".", uses == "..":
		return true
	case strings.HasPrefix(uses, "./"), strings.HasPrefix(uses, "../"), strings.HasPrefix(uses, "$/"):
		return true
	default:
		return false
	}
}

func resolveLocal(root, uses string) (string, bool) {
	u := uses
	if strings.HasPrefix(u, "$/") {
		u = "./" + strings.TrimPrefix(u, "$/")
	}
	if u == "" || filepath.IsAbs(filepath.FromSlash(u)) {
		return "", false
	}
	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(u)))
	return abs, withinRoot(root, abs)
}

func withinRoot(root, abs string) bool {
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// staysInside accepts abs when it is under root or under root's symlink target.
// Callers pass a cleaned or EvalSymlinks path; the lexical escape check uses
// withinRoot on the unevaluated path first.
func staysInside(root, abs string) bool {
	if withinRoot(root, abs) {
		return true
	}
	if real, err := filepath.EvalSymlinks(root); err == nil && withinRoot(real, abs) {
		return true
	}
	return false
}

func relUnderRoot(root, abs string) (string, error) {
	base := filepath.Clean(root)
	if !withinRoot(base, abs) {
		real, err := filepath.EvalSymlinks(root)
		if err != nil || !withinRoot(real, abs) {
			return "", fmt.Errorf("%s is outside %s", abs, root)
		}
		base = real
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func missingLocalMsg(uses string) string {
	low := strings.ToLower(uses)
	if strings.HasSuffix(low, ".yml") || strings.HasSuffix(low, ".yaml") {
		return fmt.Sprintf("local workflow %q does not exist", uses)
	}
	return fmt.Sprintf("local action %q has no action.yml", uses)
}

// externalPinIssue reports why uses is not a full lowercase commit SHA.
// An empty string means the ref is pinned.
func externalPinIssue(uses string) string {
	if strings.Count(uses, "@") != 1 {
		return fmt.Sprintf("external action %q is not pinned to a 40-character lowercase commit SHA", uses)
	}
	repo, ref, _ := strings.Cut(uses, "@")
	if !validActionRepo(repo) {
		return fmt.Sprintf("external action %q is not an owner/repo action pinned to a 40-character lowercase commit SHA", uses)
	}
	if ref == "" {
		return fmt.Sprintf("external action %q is not pinned to a 40-character lowercase commit SHA", uses)
	}
	if actionSHA.MatchString(ref) {
		return ""
	}
	if hexRef.MatchString(ref) {
		switch {
		case len(ref) == 40:
			return fmt.Sprintf("external action %q is not a lowercase commit SHA; want 40 lowercase hex characters", uses)
		case len(ref) < 40:
			return fmt.Sprintf("external action %q is a short SHA; want a 40-character lowercase commit SHA", uses)
		default:
			return fmt.Sprintf("external action %q is a %d-character SHA; want a 40-character lowercase commit SHA", uses, len(ref))
		}
	}
	return fmt.Sprintf("external action %q uses movable ref %q; want a 40-character lowercase commit SHA", uses, ref)
}

func validActionRepo(repo string) bool {
	if repo == "" || strings.Contains(repo, " ") || strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, ".") {
		return false
	}
	owner, rest, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || rest == "" {
		return false
	}
	return !strings.Contains(owner, " ")
}
