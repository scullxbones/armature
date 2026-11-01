// workflowimagepins fails closed when GitHub workflow/action files reference
// container images without a name@sha256:<64-hex> digest pin.
//
// Images collected via YAML parse (gopkg.in/yaml.v3), not line grep:
//   - jobs.*.container (string or mapping .image)
//   - jobs.*.services.*.image
//   - steps[].uses starting with docker:// (empty docker:// is unpinned)
//   - image args to docker|podman [global-flags] [container|image] run|pull|create
//   - nested sh/bash -c "docker …" strings
//
// Also scans .github/actions/**/action.yml.
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

// name@sha256:hex — bare sha256: digest alone is NOT enough.
var digestPin = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)

type imageRef struct {
	File string
	Path string
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
		fmt.Fprintln(stderr, "FAIL: workflow images must be name@sha256:<digest> pinned:")
		for _, r := range bad {
			fmt.Fprintf(stderr, "  %s (%s): %q\n", r.File, r.Path, r.Ref)
		}
		return 1
	}
	fmt.Fprintln(stdout, "OK: workflow/action images are name@sha256-pinned")
	return 0
}

func pinned(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		// Empty uses: docker://  → unpinned.
		return false
	}
	return digestPin.MatchString(ref)
}

func collectRepo(root string) ([]imageRef, error) {
	var out []imageRef
	wfDir := filepath.Join(root, ".github", "workflows")
	if entries, err := os.ReadDir(wfDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(".github", "workflows", name))
			refs, err := collectFile(filepath.Join(wfDir, name), rel)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			out = append(out, refs...)
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
		refs, err := collectActionFile(path, rel)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, refs...)
		return nil
	}); err != nil {
		return nil, err
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
	return collectWorkflowDoc(rel, doc), nil
}

func collectActionFile(absPath, rel string) ([]imageRef, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return collectActionDoc(rel, doc), nil
}

func collectWorkflowDoc(rel string, doc map[string]any) []imageRef {
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
			if services, ok := servicesRaw.(map[string]any); ok {
				for svcName, rawSvc := range services {
					svc, ok := rawSvc.(map[string]any)
					if !ok {
						continue
					}
					if img, ok := svc["image"].(string); ok {
						out = append(out, imageRef{
							File: rel,
							Path: base + ".services." + svcName + ".image",
							Ref:  strings.TrimSpace(img),
						})
					}
				}
			}
		}
		out = append(out, collectSteps(rel, base, jobSteps(job))...)
	}
	return out
}

func collectActionDoc(rel string, doc map[string]any) []imageRef {
	var out []imageRef
	if runs, ok := doc["runs"].(map[string]any); ok {
		if using, ok := runs["using"].(string); ok && using == "docker" {
			if img, ok := runs["image"].(string); ok {
				out = append(out, imageRef{
					File: rel,
					Path: "runs.image",
					Ref:  strings.TrimSpace(img),
				})
			}
		}
		out = append(out, collectSteps(rel, "runs", jobSteps(runs))...)
	}
	return out
}

func collectSteps(rel, base string, steps []map[string]any) []imageRef {
	var out []imageRef
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
	return out
}

func containerRefs(rel, path string, c any) []imageRef {
	switch v := c.(type) {
	case string:
		return []imageRef{{File: rel, Path: path, Ref: strings.TrimSpace(v)}}
	case map[string]any:
		if img, ok := v["image"].(string); ok {
			return []imageRef{{File: rel, Path: path + ".image", Ref: strings.TrimSpace(img)}}
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
	s, ok := raw.(string)
	return s, ok
}
