package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const fixtureManifest = `# fixture manifest for selftest
fixture-alpine	alpine:3.22@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`

// runSelftest requires every unpinned fixture to FAIL and every pinned twin to
// PASS against a shared fixture manifest. Both dirs must be non-empty with
// matching names and non-zero findings/refs each direction.
func runSelftest(root string, stdout, stderr io.Writer) error {
	base := filepath.Join(root, "scripts", "testdata", "workflow-image-pins")
	if err := selftestFiles(base, stdout); err != nil {
		return err
	}
	if err := selftestDirs(base, stdout); err != nil {
		return err
	}
	_ = stderr
	return nil
}

func selftestFiles(base string, stdout io.Writer) error {
	unpinnedDir := filepath.Join(base, "unpinned")
	pinnedDir := filepath.Join(base, "pinned")
	unpinnedNames, err := listYAML(unpinnedDir)
	if err != nil {
		return err
	}
	if len(unpinnedNames) == 0 {
		return fmt.Errorf("no unpinned fixtures in %s (deleting fixtures must fail)", unpinnedDir)
	}
	pinnedNames, err := listYAML(pinnedDir)
	if err != nil {
		return err
	}
	if len(pinnedNames) == 0 {
		return fmt.Errorf("no pinned fixtures in %s (deleting fixtures must fail)", pinnedDir)
	}
	if err := requireMatchingNames(unpinnedNames, pinnedNames, "pinned", "unpinned"); err != nil {
		return err
	}

	for _, name := range unpinnedNames {
		n, err := checkYAMLFixture(unpinnedDir, name, true)
		if err != nil {
			return fmt.Errorf("unpinned/%s: %w", name, err)
		}
		if n == 0 {
			return fmt.Errorf("unpinned/%s: expected findings, got 0", name)
		}
		fmt.Fprintf(stdout, "OK: unpinned/%s fails (%d findings)\n", name, n)
	}
	for _, name := range pinnedNames {
		n, err := checkYAMLFixture(pinnedDir, name, false)
		if err != nil {
			return fmt.Errorf("pinned/%s: %w", name, err)
		}
		if n != 0 {
			return fmt.Errorf("pinned/%s: expected 0 findings, got %d", name, n)
		}
		fmt.Fprintf(stdout, "OK: pinned/%s passes\n", name)
	}
	return nil
}

func selftestDirs(base string, stdout io.Writer) error {
	unpinnedRoot := filepath.Join(base, "unpinned-dirs")
	pinnedRoot := filepath.Join(base, "pinned-dirs")
	unpinnedNames, err := listDirs(unpinnedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("unpinned-dirs missing (deleting fixtures must fail)")
		}
		return err
	}
	if len(unpinnedNames) == 0 {
		return fmt.Errorf("unpinned-dirs empty (deleting fixtures must fail)")
	}
	pinnedNames, err := listDirs(pinnedRoot)
	if err != nil {
		return err
	}
	if len(pinnedNames) == 0 {
		return fmt.Errorf("pinned-dirs empty (deleting fixtures must fail)")
	}
	if err := requireMatchingNames(unpinnedNames, pinnedNames, "pinned-dirs", "unpinned-dirs"); err != nil {
		return err
	}

	for _, name := range unpinnedNames {
		n, err := checkDirFixture(filepath.Join(unpinnedRoot, name), true)
		if err != nil {
			return fmt.Errorf("unpinned-dirs/%s: %w", name, err)
		}
		if n == 0 {
			return fmt.Errorf("unpinned-dirs/%s: expected findings, got 0", name)
		}
		fmt.Fprintf(stdout, "OK: unpinned-dirs/%s fails (%d findings)\n", name, n)
	}
	for _, name := range pinnedNames {
		n, err := checkDirFixture(filepath.Join(pinnedRoot, name), false)
		if err != nil {
			return fmt.Errorf("pinned-dirs/%s: %w", name, err)
		}
		if n != 0 {
			return fmt.Errorf("pinned-dirs/%s: expected 0 findings, got %d", name, n)
		}
		fmt.Fprintf(stdout, "OK: pinned-dirs/%s passes\n", name)
	}
	return nil
}

func requireMatchingNames(a, b []string, bLabel, aLabel string) error {
	if len(a) != len(b) {
		return fmt.Errorf("%s count %d != %s count %d", bLabel, len(b), aLabel, len(a))
	}
	set := map[string]bool{}
	for _, n := range a {
		set[n] = true
	}
	for _, n := range b {
		if !set[n] {
			return fmt.Errorf("%s/%s has no %s twin", bLabel, n, aLabel)
		}
	}
	return nil
}

func checkYAMLFixture(dir, name string, expectFail bool) (n int, err error) {
	tmp, err := os.MkdirTemp("", "armature-wip-fixture-*")
	if err != nil {
		return 0, err
	}
	defer func() {
		if rmErr := os.RemoveAll(tmp); rmErr != nil && err == nil {
			err = rmErr
		}
	}()
	if err = seedFixtureRepo(tmp); err != nil {
		return 0, err
	}
	wfDir := filepath.Join(tmp, ".github", "workflows")
	if err = os.MkdirAll(wfDir, 0o755); err != nil {
		return 0, err
	}
	src := filepath.Join(dir, name)
	dst := filepath.Join(wfDir, "ci.yml")
	if err = copyFile(src, dst); err != nil {
		return 0, err
	}
	findings, err := checkRepo(tmp)
	if err != nil {
		return 0, err
	}
	_ = expectFail
	return len(findings), nil
}

func checkDirFixture(srcRoot string, expectFail bool) (n int, err error) {
	tmp, err := os.MkdirTemp("", "armature-wip-dirfix-*")
	if err != nil {
		return 0, err
	}
	defer func() {
		if rmErr := os.RemoveAll(tmp); rmErr != nil && err == nil {
			err = rmErr
		}
	}()
	if err = seedFixtureRepo(tmp); err != nil {
		return 0, err
	}
	// Copy fixture tree over the seeded repo (preserves .github/, Dockerfile, action.yml).
	if err = copyTree(srcRoot, tmp); err != nil {
		return 0, err
	}
	findings, err := checkRepo(tmp)
	if err != nil {
		return 0, err
	}
	_ = expectFail
	return len(findings), nil
}

func seedFixtureRepo(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(manifestRel)), []byte(fixtureManifest), 0o644); err != nil {
		return err
	}
	// Minimal wrapper so allowlist path exists; must contain docker (it is the sole allowlist).
	wrapper := "#!/usr/bin/env bash\nset -euo pipefail\nexec docker run \"$@\"\n"
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(wrapperRel)), []byte(wrapper), 0o755)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func listYAML(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml") {
			out = append(out, n)
		}
	}
	return out, nil
}

func listDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
