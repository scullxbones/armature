package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
)

const golangciInTestsFix = "FAIL: _test.go must not exec golangci-lint; use scripts/repoguard -check gochecknoglobals-probe (make lint) instead:"

// checkGolangciInTests fails when any *_test.go shells golangci-lint via
// os/exec.Command, CommandContext, or LookPath. Lint probes belong in
// repoguard / make lint, not go test.
func checkGolangciInTests(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	files, fset, err := parseGoTree(root, true)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	var hits []string
	for _, pf := range files {
		hits = append(hits, scanGolangciInTestFile(fset, pf)...)
	}
	if len(hits) > 0 {
		fmt.Fprintln(stderr, golangciInTestsFix)
		for _, h := range hits {
			fmt.Fprintln(stderr, h)
		}
		return fmt.Errorf("golangci-lint in tests")
	}
	fmt.Fprintln(stdout, "OK: no _test.go execs golangci-lint")
	return nil
}

func scanGolangciInTestFile(fset *token.FileSet, pf parsedFile) []string {
	fileNums, fileStrs := collectFileConsts(pf.file, pf.imports)
	var hits []string
	ast.Inspect(pf.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GenDecl:
			if x.Tok == token.CONST || x.Tok == token.VAR {
				bindValueSpecs(x.Specs, fileNums, fileStrs, fileNums, fileStrs, pf.imports)
			}
		case *ast.AssignStmt:
			bindAssign(x, fileNums, fileStrs, pf.imports)
		case *ast.CallExpr:
			if golangciLintExec(x, pf.imports, fileStrs) {
				hits = append(hits, formatHit(fset, x.Pos(), pf.rel, "execs golangci-lint"))
			}
		}
		return true
	})
	return hits
}

func golangciLintExec(call *ast.CallExpr, imports map[string]string, env map[string]string) bool {
	args := foldedArgs(call, env)
	switch {
	case sel(call.Fun, imports, "os/exec", "Command"):
		return len(args) > 0 && args[0] == golangciBinary
	case sel(call.Fun, imports, "os/exec", "CommandContext"):
		return len(args) > 1 && args[1] == golangciBinary
	case sel(call.Fun, imports, "os/exec", "LookPath"):
		return len(args) > 0 && args[0] == golangciBinary
	default:
		return false
	}
}
