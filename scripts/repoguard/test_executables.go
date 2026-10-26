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

type chmodEvent struct {
	pos    token.Pos
	kind   string
	mode   ast.Expr
	path   string
	method bool
}

func scanTestExecFunc(fset *token.FileSet, pf parsedFile, fn *ast.FuncDecl, fileNums map[string]uint64) []string {
	nums := map[string]uint64{}
	for k, v := range fileNums {
		nums[k] = v
	}
	written := map[string]bool{}
	var hits []string
	var chmods []chmodEvent
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
			hits = append(hits, noteExecWrite(fset, pf, stmt, nums, written)...)
			if ev, ok := noteExecChmod(pf, stmt, nums); ok {
				chmods = append(chmods, ev)
			}
		}
		return true
	})
	for _, ev := range chmods {
		if ev.method {
			if len(written) == 0 {
				continue
			}
		} else if !written[ev.path] {
			continue
		}
		hits = append(hits, formatExecHit(fset, ev.pos, pf.rel, ev.kind, ev.mode))
	}
	return hits
}

func modeLooksExecutable(expr ast.Expr, nums map[string]uint64, imports map[string]string) bool {
	mode, ok := evalUint(expr, nums, imports)
	return !ok || hasUnixExec(mode)
}

func noteExecWrite(fset *token.FileSet, pf parsedFile, call *ast.CallExpr, nums map[string]uint64, written map[string]bool) []string {
	var kind string
	var mode ast.Expr
	switch {
	case sel(call.Fun, pf.imports, "os", "WriteFile") && len(call.Args) >= 3:
		kind, mode = "os.WriteFile", call.Args[2]
	case sel(call.Fun, pf.imports, "os", "OpenFile") && len(call.Args) >= 3:
		kind, mode = "os.OpenFile", call.Args[2]
	default:
		return nil
	}
	written[pathKey(call.Args[0])] = true
	if !modeLooksExecutable(mode, nums, pf.imports) {
		return nil
	}
	return []string{formatExecHit(fset, call.Pos(), pf.rel, kind, mode)}
}

func noteExecChmod(pf parsedFile, call *ast.CallExpr, nums map[string]uint64) (chmodEvent, bool) {
	if sel(call.Fun, pf.imports, "os", "Chmod") {
		if len(call.Args) < 2 || !modeLooksExecutable(call.Args[1], nums, pf.imports) {
			return chmodEvent{}, false
		}
		return chmodEvent{
			pos:  call.Pos(),
			kind: "os.Chmod",
			mode: call.Args[1],
			path: pathKey(call.Args[0]),
		}, true
	}
	s, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || s.Sel == nil || s.Sel.Name != "Chmod" || len(call.Args) < 1 {
		return chmodEvent{}, false
	}
	if !modeLooksExecutable(call.Args[0], nums, pf.imports) {
		return chmodEvent{}, false
	}
	return chmodEvent{
		pos:    call.Pos(),
		kind:   "Chmod",
		mode:   call.Args[0],
		method: true,
	}, true
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
