package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runSelftest requires every unpinned fixture under
// scripts/testdata/workflow-image-pins/unpinned/ to FAIL the pin check, and
// every pinned twin under .../pinned/ to PASS.
func runSelftest(root string, stdout, stderr io.Writer) error {
	base := filepath.Join(root, "scripts", "testdata", "workflow-image-pins")
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

	for _, name := range unpinnedNames {
		path := filepath.Join(unpinnedDir, name)
		refs, err := collectFile(path, "unpinned/"+name)
		if err != nil {
			return fmt.Errorf("unpinned/%s: %w", name, err)
		}
		if len(refs) == 0 {
			return fmt.Errorf("unpinned/%s: collected zero image refs (fixture must exercise a form)", name)
		}
		var bad int
		for _, ref := range refs {
			if !pinned(ref.Ref) {
				bad++
			}
		}
		if bad == 0 {
			return fmt.Errorf("unpinned/%s: expected at least one unpinned ref, got all pinned: %v", name, refs)
		}
		fmt.Fprintf(stdout, "OK: unpinned/%s fails pin check (%d unpinned ref(s))\n", name, bad)
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
		fmt.Fprintf(stdout, "OK: pinned/%s passes pin check (%d ref(s))\n", name, len(refs))
	}

	// Names must match across twins.
	uSet := map[string]bool{}
	for _, n := range unpinnedNames {
		uSet[n] = true
	}
	for _, n := range pinnedNames {
		if !uSet[n] {
			return fmt.Errorf("pinned/%s has no unpinned twin", n)
		}
	}
	_ = stderr
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
