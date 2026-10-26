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
	fileNums, _ := collectFileConsts(pf.file, pf.imports)
	var hits []string
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		hits = append(hits, scanTestExecFunc(fset, pf, fn, fileNums)...)
	}
	return hits
}

func scanTestExecFunc(fset *token.FileSet, pf parsedFile, fn *ast.FuncDecl, fileNums map[string]uint64) []string {
	nums := map[string]uint64{}
	for k, v := range fileNums {
		nums[k] = v
	}
	var hits []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.DeclStmt:
			if gen, ok := stmt.Decl.(*ast.GenDecl); ok && (gen.Tok == token.CONST || gen.Tok == token.VAR) {
				bindValueSpecs(gen.Specs, nums, map[string]string{}, nums, map[string]string{}, pf.imports)
			}
		case *ast.AssignStmt:
			bindAssign(stmt, nums, map[string]string{}, pf.imports)
		case *ast.CallExpr:
			if hit, ok := noteExecModeCall(fset, pf, stmt, nums); ok {
				hits = append(hits, hit)
			}
		}
		return true
	})
	return hits
}

func modeLooksExecutable(expr ast.Expr, nums map[string]uint64, imports map[string]string) bool {
	mode, ok := evalUint(expr, nums, imports)
	return !ok || hasUnixExec(mode)
}

func noteExecModeCall(fset *token.FileSet, pf parsedFile, call *ast.CallExpr, nums map[string]uint64) (string, bool) {
	kind, mode, ok := execModeArg(call, pf.imports)
	if !ok || !modeLooksExecutable(mode, nums, pf.imports) {
		return "", false
	}
	return formatExecHit(fset, call.Pos(), pf.rel, kind, mode), true
}

func execModeArg(call *ast.CallExpr, imports map[string]string) (kind string, mode ast.Expr, ok bool) {
	switch {
	case sel(call.Fun, imports, "os", "WriteFile") && len(call.Args) >= 3:
		return "os.WriteFile", call.Args[2], true
	case sel(call.Fun, imports, "os", "OpenFile") && len(call.Args) >= 3:
		return "os.OpenFile", call.Args[2], true
	case sel(call.Fun, imports, "os", "Chmod") && len(call.Args) >= 2:
		return "os.Chmod", call.Args[1], true
	}
	s, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || s.Sel == nil || s.Sel.Name != "Chmod" || len(call.Args) < 1 {
		return "", nil, false
	}
	return "Chmod", call.Args[0], true
}

func formatExecHit(fset *token.FileSet, pos token.Pos, rel, kind string, mode ast.Expr) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s %s", rel, p.Line, kind, exprText(mode))
}
