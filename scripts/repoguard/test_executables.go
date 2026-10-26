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
	type pkgExec struct {
		uf      *objUF
		written map[string]bool
		chmods  []chmodEvent
		hits    []string
		funcs   map[string]*fnObjs
	}
	byPkg := map[string]*pkgExec{}
	for _, pf := range files {
		if underDir(pf.rel, "internal/testexec") {
			continue
		}
		key := pkgKey(pf.rel, pf.pkg)
		pe := byPkg[key]
		if pe == nil {
			pe = &pkgExec{
				uf:      newObjUF(),
				written: map[string]bool{},
				funcs:   map[string]*fnObjs{},
			}
			byPkg[key] = pe
		}
		indexTestExecFuncs(pe.funcs, key, pf)
	}
	for _, pf := range files {
		if underDir(pf.rel, "internal/testexec") {
			continue
		}
		key := pkgKey(pf.rel, pf.pkg)
		pe := byPkg[key]
		hits, written, chmods := scanTestExecFile(fset, pf, key, pe.funcs, pe.uf)
		pe.hits = append(pe.hits, hits...)
		for id := range written {
			pe.written[id] = true
		}
		pe.chmods = append(pe.chmods, chmods...)
	}
	var hits []string
	for _, pe := range byPkg {
		hits = append(hits, pe.hits...)
		written := map[string]bool{}
		for id := range pe.written {
			written[pe.uf.find(id)] = true
		}
		for _, ev := range pe.chmods {
			if ev.method {
				hits = append(hits, formatExecHit(fset, ev.pos, ev.rel, ev.kind, ev.mode))
				continue
			}
			if !written[pe.uf.find(ev.obj)] {
				continue
			}
			hits = append(hits, formatExecHit(fset, ev.pos, ev.rel, ev.kind, ev.mode))
		}
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

type fnObjs struct {
	id     string
	locals map[string]bool
	params []string
}

type chmodEvent struct {
	pos    token.Pos
	kind   string
	mode   ast.Expr
	obj    string
	rel    string
	method bool
}

type objUF struct {
	parent map[string]string
}

func newObjUF() *objUF {
	return &objUF{parent: map[string]string{}}
}

func (u *objUF) find(x string) string {
	if x == "" {
		return ""
	}
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	if u.parent[x] != x {
		u.parent[x] = u.find(u.parent[x])
	}
	return u.parent[x]
}

func (u *objUF) union(a, b string) {
	if a == "" || b == "" {
		return
	}
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

func indexTestExecFuncs(dst map[string]*fnObjs, pkg string, pf parsedFile) {
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		id := pkg + ":" + fn.Name.Name
		dst[fn.Name.Name] = &fnObjs{id: id, locals: funcLocals(fn), params: funcParams(fn)}
	}
}

func funcParams(fn *ast.FuncDecl) []string {
	var names []string
	if fn.Type == nil || fn.Type.Params == nil {
		return names
	}
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n != nil && n.Name != "_" {
				names = append(names, n.Name)
			}
		}
	}
	return names
}

func funcLocals(fn *ast.FuncDecl) map[string]bool {
	locals := map[string]bool{}
	for _, n := range funcParams(fn) {
		locals[n] = true
	}
	if fn.Body == nil {
		return locals
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range stmt.Lhs {
				id, ok := lhs.(*ast.Ident)
				if ok && id.Name != "_" {
					locals[id.Name] = true
				}
			}
		case *ast.DeclStmt:
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range vs.Names {
					if name != nil && name.Name != "_" {
						locals[name.Name] = true
					}
				}
			}
		}
		return true
	})
	return locals
}

func scanTestExecFile(fset *token.FileSet, pf parsedFile, pkg string, funcs map[string]*fnObjs, uf *objUF) (hits []string, written map[string]bool, chmods []chmodEvent) {
	fileNums, fileStrs := collectFileConsts(pf.file, pf.imports)
	written = map[string]bool{}
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		fo := (*fnObjs)(nil)
		if fn.Name != nil {
			fo = funcs[fn.Name.Name]
		}
		if fo == nil {
			fo = &fnObjs{id: pkg + ":_", locals: funcLocals(fn), params: funcParams(fn)}
		}
		fnHits, fnWritten, fnChmods := scanTestExecFunc(fset, pf, fn, fo, pkg, fileNums, fileStrs, funcs, uf)
		hits = append(hits, fnHits...)
		for id := range fnWritten {
			written[id] = true
		}
		chmods = append(chmods, fnChmods...)
	}
	return hits, written, chmods
}

func scanTestExecFunc(
	fset *token.FileSet,
	pf parsedFile,
	fn *ast.FuncDecl,
	fo *fnObjs,
	pkg string,
	fileNums map[string]uint64,
	fileStrs map[string]string,
	funcs map[string]*fnObjs,
	uf *objUF,
) ([]string, map[string]bool, []chmodEvent) {
	nums := map[string]uint64{}
	for k, v := range fileNums {
		nums[k] = v
	}
	strs := map[string]string{}
	for k, v := range fileStrs {
		strs[k] = v
	}
	written := map[string]bool{}
	var hits []string
	var chmods []chmodEvent
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.DeclStmt:
			if gen, ok := stmt.Decl.(*ast.GenDecl); ok && (gen.Tok == token.CONST || gen.Tok == token.VAR) {
				bindValueSpecs(gen.Specs, nums, strs, nums, strs, pf.imports)
			}
		case *ast.AssignStmt:
			bindAssign(stmt, nums, strs, pf.imports)
			aliasAssign(stmt, fo, pkg, strs, uf, written, pf)
		case *ast.CallExpr:
			hits = append(hits, noteExecWrite(fset, pf, stmt, fo, pkg, nums, strs, written)...)
			if ev, ok := noteExecChmod(pf, stmt, fo, pkg, nums, strs); ok {
				chmods = append(chmods, ev)
			}
			aliasCall(stmt, fo, pkg, strs, funcs, uf)
		}
		return true
	})
	return hits, written, chmods
}

func aliasAssign(stmt *ast.AssignStmt, fo *fnObjs, pkg string, strs map[string]string, uf *objUF, written map[string]bool, pf parsedFile) {
	for i, lhs := range stmt.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" || i >= len(stmt.Rhs) {
			continue
		}
		left := pathObj(pkg, fo, strs, id)
		right := stmt.Rhs[i]
		uf.union(left, pathObj(pkg, fo, strs, right))
		if call, ok := right.(*ast.CallExpr); ok && sel(call.Fun, pf.imports, "os", "OpenFile") && len(call.Args) >= 1 {
			uf.union(left, pathObj(pkg, fo, strs, call.Args[0]))
			written[left] = true
		}
	}
}

func aliasCall(call *ast.CallExpr, fo *fnObjs, pkg string, strs map[string]string, funcs map[string]*fnObjs, uf *objUF) {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name == "" {
		return
	}
	callee, ok := funcs[id.Name]
	if !ok {
		return
	}
	for i, arg := range call.Args {
		if i >= len(callee.params) {
			break
		}
		param := pathObj(pkg, callee, strs, &ast.Ident{Name: callee.params[i]})
		uf.union(pathObj(pkg, fo, strs, arg), param)
	}
}

func pathObj(pkg string, fo *fnObjs, strs map[string]string, expr ast.Expr) string {
	if s, ok := foldString(expr, strs); ok {
		return "s:" + pkg + ":" + s
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if fo != nil && fo.locals[e.Name] {
			return "l:" + fo.id + ":" + e.Name
		}
		return "p:" + pkg + ":" + e.Name
	case *ast.ParenExpr:
		return pathObj(pkg, fo, strs, e.X)
	default:
		fnID := pkg
		if fo != nil {
			fnID = fo.id
		}
		return "x:" + fnID + ":" + exprText(e)
	}
}

func modeLooksExecutable(expr ast.Expr, nums map[string]uint64, imports map[string]string) bool {
	mode, ok := evalUint(expr, nums, imports)
	return !ok || hasUnixExec(mode)
}

func noteExecWrite(
	fset *token.FileSet,
	pf parsedFile,
	call *ast.CallExpr,
	fo *fnObjs,
	pkg string,
	nums map[string]uint64,
	strs map[string]string,
	written map[string]bool,
) []string {
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
	written[pathObj(pkg, fo, strs, call.Args[0])] = true
	if !modeLooksExecutable(mode, nums, pf.imports) {
		return nil
	}
	return []string{formatExecHit(fset, call.Pos(), pf.rel, kind, mode)}
}

func noteExecChmod(pf parsedFile, call *ast.CallExpr, fo *fnObjs, pkg string, nums map[string]uint64, strs map[string]string) (chmodEvent, bool) {
	if sel(call.Fun, pf.imports, "os", "Chmod") {
		if len(call.Args) < 2 || !modeLooksExecutable(call.Args[1], nums, pf.imports) {
			return chmodEvent{}, false
		}
		return chmodEvent{
			pos:  call.Pos(),
			kind: "os.Chmod",
			mode: call.Args[1],
			obj:  pathObj(pkg, fo, strs, call.Args[0]),
			rel:  pf.rel,
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
		obj:    pathObj(pkg, fo, strs, s.X),
		rel:    pf.rel,
		method: true,
	}, true
}

func formatExecHit(fset *token.FileSet, pos token.Pos, rel, kind string, mode ast.Expr) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s %s", rel, p.Line, kind, exprText(mode))
}
