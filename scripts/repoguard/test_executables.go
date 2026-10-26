package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
)

const testExecFix = "FAIL: _test.go writes an executable; use testexec.Write so tests never create an executable inode after init (golang/go#22315):"

func checkTestExecutables(root string, stdout, stderr io.Writer) error {
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
		if underDir(pf.rel, "internal/testexec") {
			continue
		}
		hits = append(hits, scanTestExecFile(fset, pf)...)
	}
	if len(hits) > 0 {
		fmt.Fprintln(stderr, testExecFix)
		for _, h := range hits {
			fmt.Fprintln(stderr, h)
		}
		return fmt.Errorf("test executable write")
	}
	fmt.Fprintln(stdout, "OK: no _test.go writes an executable outside testexec.Write")
	return nil
}

func scanTestExecFile(fset *token.FileSet, pf parsedFile) []string {
	var hits []string
	ast.Inspect(pf.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if hit, ok := noteExecModeCall(fset, pf, call); ok {
			hits = append(hits, hit)
		}
		return true
	})
	return hits
}

func modeLooksExecutable(expr ast.Expr, imports map[string]string) bool {
	mode, ok := evalUint(expr, map[string]uint64{}, imports)
	return !ok || hasUnixExec(mode)
}

func noteExecModeCall(fset *token.FileSet, pf parsedFile, call *ast.CallExpr) (string, bool) {
	kind, mode, ok := execModeArg(call, pf.imports)
	if !ok || !modeLooksExecutable(mode, pf.imports) {
		return "", false
	}
	return formatExecHit(fset, call.Pos(), pf.rel, kind, mode), true
}

func execModeArg(call *ast.CallExpr, imports map[string]string) (kind string, mode ast.Expr, ok bool) {
	n := len(call.Args)
	switch {
	case sel(call.Fun, imports, "os", "WriteFile") && n >= 3:
		return "os.WriteFile", call.Args[2], true
	case sel(call.Fun, imports, "io/ioutil", "WriteFile") && n >= 3:
		return "ioutil.WriteFile", call.Args[2], true
	case sel(call.Fun, imports, "os", "OpenFile") && n >= 3:
		return "os.OpenFile", call.Args[2], true
	case sel(call.Fun, imports, "os", "Chmod") && n >= 2:
		return "os.Chmod", call.Args[1], true
	case sel(call.Fun, imports, "syscall", "Chmod") && n >= 2:
		return "syscall.Chmod", call.Args[1], true
	case sel(call.Fun, imports, "golang.org/x/sys/unix", "Chmod") && n >= 2:
		return "unix.Chmod", call.Args[1], true
	}
	s, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || s.Sel == nil || s.Sel.Name != "Chmod" || n < 1 {
		return "", nil, false
	}
	return "Chmod", call.Args[0], true
}

func formatExecHit(fset *token.FileSet, pos token.Pos, rel, kind string, mode ast.Expr) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s %s", rel, p.Line, kind, exprText(mode))
}
