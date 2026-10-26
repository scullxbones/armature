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
	fileNums := collectFileUintConsts(pf.file, pf.imports)
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
	nums := onceAssignedNums(fn, fileNums, pf.imports)
	var hits []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if hit, ok := noteExecModeCall(fset, pf, call, nums); ok {
			hits = append(hits, hit)
		}
		return true
	})
	return hits
}

func collectFileUintConsts(file *ast.File, imports map[string]string) map[string]uint64 {
	nums := map[string]uint64{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		bindValueSpecs(gen.Specs, nums, map[string]string{}, nums, map[string]string{}, imports)
	}
	return nums
}

func onceAssignedNums(fn *ast.FuncDecl, fileNums map[string]uint64, imports map[string]string) map[string]uint64 {
	counts := map[string]int{}
	rhs := map[string]ast.Expr{}
	note := func(name string, val ast.Expr) {
		if name == "" || name == "_" {
			return
		}
		counts[name]++
		rhs[name] = val
	}
	if fn.Type != nil && fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, n := range field.Names {
				if n != nil {
					note(n.Name, nil)
				}
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		switch stmt := n.(type) {
		case *ast.DeclStmt:
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if name == nil {
						continue
					}
					var val ast.Expr
					if i < len(vs.Values) {
						val = vs.Values[i]
					}
					note(name.Name, val)
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				var val ast.Expr
				if i < len(stmt.Rhs) {
					val = stmt.Rhs[i]
				}
				note(id.Name, val)
			}
		}
		return true
	})
	nums := map[string]uint64{}
	for k, v := range fileNums {
		nums[k] = v
	}
	for name, n := range counts {
		delete(nums, name)
		if n != 1 || rhs[name] == nil {
			continue
		}
		if u, ok := evalUint(rhs[name], fileNums, imports); ok {
			nums[name] = u
		}
	}
	return nums
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
