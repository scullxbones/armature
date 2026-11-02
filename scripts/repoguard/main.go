// Repo guards over Go syntax and constant-folded values, not rg/awk text.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	check := "all"
	root := "."
	for len(args) > 0 {
		switch args[0] {
		case "-check":
			if len(args) < 2 {
				fmt.Fprintf(stderr, "FAIL: -check requires source-manifest|test-executables|git-hermetic|"+
					"gochecknoglobals-probe|golangci-in-tests|object-id|residual-trls|git-network|all\n")
				return 2
			}
			check = args[1]
			args = args[2:]
		case "-h", "-help", "--help":
			fmt.Fprintf(stderr, "usage: repoguard [-check source-manifest|test-executables|"+
				"git-hermetic|gochecknoglobals-probe|golangci-in-tests|object-id|residual-trls|git-network|all] [repo-root]\n")
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
	root = abs

	var failed bool
	runOne := func(name string, fn func(string, io.Writer, io.Writer) error) {
		if check != "all" && check != name {
			return
		}
		if err := fn(root, stdout, stderr); err != nil {
			failed = true
		}
	}
	runOne("source-manifest", checkSourceManifest)
	runOne("test-executables", checkTestExecutables)
	runOne("git-hermetic", checkGitHermetic)
	runOne("gochecknoglobals-probe", checkGochecknoglobalsProbe)
	runOne("golangci-in-tests", checkGolangciInTests)
	runOne("object-id", checkObjectID)
	runOne("residual-trls", checkResidualTrls)
	runOne("git-network", checkGitNetwork)
	known := check == "all" ||
		check == "source-manifest" ||
		check == "test-executables" ||
		check == "git-hermetic" ||
		check == "gochecknoglobals-probe" ||
		check == "golangci-in-tests" ||
		check == "object-id" ||
		check == "residual-trls" ||
		check == "git-network"
	if !known {
		fmt.Fprintf(stderr, "FAIL: unknown check %s\n", check)
		return 2
	}
	if failed {
		return 1
	}
	return 0
}
