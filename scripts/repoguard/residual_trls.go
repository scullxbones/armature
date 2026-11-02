package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const residualTrlsFix = "Rename residual pre-rename CLI token trls → arm. " +
	"History (changelog, ADRs, archive, migration notes) may use an allowlist entry in allow.go."

// residualTrlsWord matches the pre-rename CLI token as a whole word so
// substrings like "controls" do not false-positive.
var residualTrlsWord = regexp.MustCompile(`(?i)\btrls\b`)

func checkResidualTrls(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	var hits []string
	for _, dir := range []string{"cmd", "internal", "docs/assets"} {
		dirHits, err := scanResidualTrlsDir(root, dir)
		if err != nil {
			fmt.Fprintf(stderr, "FAIL: %v\n", err)
			return err
		}
		hits = append(hits, dirHits...)
	}
	if len(hits) > 0 {
		for _, h := range hits {
			fmt.Fprintln(stderr, "FAIL: "+h)
		}
		fmt.Fprintln(stderr, residualTrlsFix)
		return fmt.Errorf("residual trls token")
	}
	fmt.Fprintln(stdout, "OK: residual trls absent under cmd/, internal/, docs/assets/ (allowlist for history)")
	return nil
}

func scanResidualTrlsDir(root, relDir string) ([]string, error) {
	abs := filepath.Join(root, filepath.FromSlash(relDir))
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hits []string
	err := filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if name == "testdata" || name == "vendor" || name == ".git" || strings.HasPrefix(name, ".") {
				if path == abs {
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if allowed(rel, allowResidualTrls) {
			return nil
		}
		// Product + test Go under cmd/internal; asciinema casts under docs/assets.
		switch {
		case strings.HasSuffix(rel, ".go"):
		case strings.HasSuffix(rel, ".cast"):
		default:
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !residualTrlsWord.Match(data) {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if residualTrlsWord.MatchString(line) {
				hits = append(hits, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	return hits, err
}
