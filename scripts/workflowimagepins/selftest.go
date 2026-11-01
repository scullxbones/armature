package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runSelftest requires every unpinned fixture to FAIL the pin check and every
// pinned twin to PASS. Supports:
//   - single-file workflows under unpinned/ and pinned/
//   - mini-repo dirs under unpinned-dirs/ and pinned-dirs/ (for action.yml)
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
		return fmt.Errorf("no unpinned fixtures in %s", unpinnedDir)
	}
	pinnedNames, err := listYAML(pinnedDir)
	if err != nil {
		return err
	}
	if len(pinnedNames) != len(unpinnedNames) {
		return fmt.Errorf("pinned twin count %d != unpinned count %d", len(pinnedNames), len(unpinnedNames))
	}
	uSet := map[string]bool{}
	for _, n := range unpinnedNames {
		uSet[n] = true
	}
	for _, n := range pinnedNames {
		if !uSet[n] {
			return fmt.Errorf("pinned/%s has no unpinned twin", n)
		}
	}

	for _, name := range unpinnedNames {
		path := filepath.Join(unpinnedDir, name)
		refs, err := collectFile(path, "unpinned/"+name)
		if err != nil {
			return fmt.Errorf("unpinned/%s: %w", name, err)
		}
		if len(refs) == 0 {
			return fmt.Errorf("unpinned/%s: collected zero image refs", name)
		}
		bad := 0
		for _, ref := range refs {
			if !pinned(ref.Ref) {
				bad++
			}
		}
		if bad == 0 {
			return fmt.Errorf("unpinned/%s: expected unpinned refs, got all pinned: %v", name, refs)
		}
		fmt.Fprintf(stdout, "OK: unpinned/%s fails (%d unpinned)\n", name, bad)
	}
	for _, name := range pinnedNames {
		path := filepath.Join(pinnedDir, name)
		refs, err := collectFile(path, "pinned/"+name)
		if err != nil {
			return fmt.Errorf("pinned/%s: %w", name, err)
		}
		if len(refs) == 0 {
			return fmt.Errorf("pinned/%s: collected zero image refs", name)
		}
		for _, ref := range refs {
			if !pinned(ref.Ref) {
				return fmt.Errorf("pinned/%s: still unpinned %s %q", name, ref.Path, ref.Ref)
			}
		}
		fmt.Fprintf(stdout, "OK: pinned/%s passes (%d refs)\n", name, len(refs))
	}
	return nil
}

func selftestDirs(base string, stdout io.Writer) error {
	unpinnedRoot := filepath.Join(base, "unpinned-dirs")
	pinnedRoot := filepath.Join(base, "pinned-dirs")
	unpinnedNames, err := listDirs(unpinnedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pinnedNames, err := listDirs(pinnedRoot)
	if err != nil {
		return err
	}
	if len(pinnedNames) != len(unpinnedNames) {
		return fmt.Errorf("pinned-dirs twin count %d != unpinned-dirs %d", len(pinnedNames), len(unpinnedNames))
	}
	for _, name := range unpinnedNames {
		refs, err := collectRepo(filepath.Join(unpinnedRoot, name))
		if err != nil {
			return fmt.Errorf("unpinned-dirs/%s: %w", name, err)
		}
		if len(refs) == 0 {
			return fmt.Errorf("unpinned-dirs/%s: zero refs", name)
		}
		bad := 0
		for _, ref := range refs {
			if !pinned(ref.Ref) {
				bad++
			}
		}
		if bad == 0 {
			return fmt.Errorf("unpinned-dirs/%s: expected failures", name)
		}
		fmt.Fprintf(stdout, "OK: unpinned-dirs/%s fails (%d unpinned)\n", name, bad)
	}
	for _, name := range pinnedNames {
		refs, err := collectRepo(filepath.Join(pinnedRoot, name))
		if err != nil {
			return fmt.Errorf("pinned-dirs/%s: %w", name, err)
		}
		for _, ref := range refs {
			if !pinned(ref.Ref) {
				return fmt.Errorf("pinned-dirs/%s: still unpinned %q", name, ref.Ref)
			}
		}
		fmt.Fprintf(stdout, "OK: pinned-dirs/%s passes (%d refs)\n", name, len(refs))
	}
	return nil
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
