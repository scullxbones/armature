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
	written := map[string]bool{}
	var hits []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.DeclStmt:
			if gen, ok := stmt.Decl.(*ast.GenDecl); ok && (gen.Tok == token.CONST || gen.Tok == token.VAR) {
				strs := map[string]string{}
				bindValueSpecs(gen.Specs, nums, strs, nums, strs, pf.imports)
			}
		case *ast.AssignStmt:
			bindAssign(stmt, nums, map[string]string{}, pf.imports)
		case *ast.CallExpr:
			hits = append(hits, noteExecCall(fset, pf, stmt, nums, written)...)
		}
		return true
	})
	return hits
}

func noteExecCall(fset *token.FileSet, pf parsedFile, call *ast.CallExpr, nums map[string]uint64, written map[string]bool) []string {
	var hits []string
	switch {
	case sel(call.Fun, pf.imports, "os", "WriteFile") && len(call.Args) >= 3:
		written[pathKey(call.Args[0])] = true
		if mode, ok := evalUint(call.Args[2], nums, pf.imports); ok && hasUnixExec(mode) {
			hits = append(hits, formatExecHit(fset, call.Pos(), pf.rel, "os.WriteFile", call.Args[2]))
		}
	case sel(call.Fun, pf.imports, "os", "OpenFile") && len(call.Args) >= 3:
		written[pathKey(call.Args[0])] = true
		if mode, ok := evalUint(call.Args[2], nums, pf.imports); ok && hasUnixExec(mode) {
			hits = append(hits, formatExecHit(fset, call.Pos(), pf.rel, "os.OpenFile", call.Args[2]))
		}
	case sel(call.Fun, pf.imports, "os", "Chmod") && len(call.Args) >= 2:
		if mode, ok := evalUint(call.Args[1], nums, pf.imports); ok && hasUnixExec(mode) && written[pathKey(call.Args[0])] {
			hits = append(hits, formatExecHit(fset, call.Pos(), pf.rel, "os.Chmod", call.Args[1]))
		}
	default:
		s, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || s.Sel == nil || s.Sel.Name != "Chmod" || len(call.Args) < 1 {
			return hits
		}
		if sel(call.Fun, pf.imports, "os", "Chmod") {
			return hits
		}
		if mode, ok := evalUint(call.Args[0], nums, pf.imports); ok && hasUnixExec(mode) && len(written) > 0 {
			hits = append(hits, formatExecHit(fset, call.Pos(), pf.rel, "Chmod", call.Args[0]))
		}
	}
	return hits
}

func pathKey(expr ast.Expr) string {
	if s, ok := foldString(expr, nil); ok {
		return "s:" + s
	}
	return "x:" + exprText(expr)
}

func formatExecHit(fset *token.FileSet, pos token.Pos, rel, kind string, mode ast.Expr) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s %s", rel, p.Line, kind, exprText(mode))
}
