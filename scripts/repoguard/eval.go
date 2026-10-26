package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"strconv"
	"strings"
)

func strconvUnquote(v string) (string, error) {
	return strconv.Unquote(v)
}

func foldString(expr ast.Expr, env map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := env[e.Name]
		return s, ok
	case *ast.ParenExpr:
		return foldString(e.X, env)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, okL := foldString(e.X, env)
		right, okR := foldString(e.Y, env)
		if !okL || !okR {
			return "", false
		}
		return left + right, true
	default:
		return "", false
	}
}

func evalUint(expr ast.Expr, env map[string]uint64, imports map[string]string) (uint64, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		c := constant.MakeFromLiteral(e.Value, token.INT, 0)
		if c.Kind() != constant.Int {
			return 0, false
		}
		u, ok := constant.Uint64Val(c)
		return u, ok
	case *ast.Ident:
		u, ok := env[e.Name]
		return u, ok
	case *ast.ParenExpr:
		return evalUint(e.X, env, imports)
	case *ast.UnaryExpr:
		v, ok := evalUint(e.X, env, imports)
		if !ok {
			return 0, false
		}
		if e.Op == token.XOR {
			return ^v, true
		}
		if e.Op == token.ADD {
			return v, true
		}
		return 0, false
	case *ast.BinaryExpr:
		x, okX := evalUint(e.X, env, imports)
		y, okY := evalUint(e.Y, env, imports)
		if !okX || !okY {
			return 0, false
		}
		switch e.Op {
		case token.OR:
			return x | y, true
		case token.AND:
			return x & y, true
		case token.XOR:
			return x ^ y, true
		case token.SHL:
			return x << y, true
		case token.SHR:
			return x >> y, true
		case token.ADD:
			return x + y, true
		case token.SUB:
			return x - y, true
		default:
			return 0, false
		}
	case *ast.CallExpr:
		if len(e.Args) != 1 {
			return 0, false
		}
		if isFileModeConv(e.Fun, imports) {
			return evalUint(e.Args[0], env, imports)
		}
		return 0, false
	case *ast.SelectorExpr:
		if e.Sel != nil && e.Sel.Name == "ModePerm" && isPkg(e.X, imports, "os", "io/fs") {
			return 0o777, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func hasUnixExec(mode uint64) bool {
	return mode&0o111 != 0
}

func isFileModeConv(fun ast.Expr, imports map[string]string) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "FileMode"
	case *ast.SelectorExpr:
		return f.Sel != nil && f.Sel.Name == "FileMode" && isPkg(f.X, imports, "os", "io/fs")
	default:
		return false
	}
}

func isPkg(expr ast.Expr, imports map[string]string, paths ...string) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	if p, ok := imports[ident.Name]; ok {
		for _, want := range paths {
			if p == want {
				return true
			}
		}
		return false
	}
	for _, want := range paths {
		if ident.Name == pathBase(want) {
			return true
		}
	}
	return false
}

func sel(fun ast.Expr, imports map[string]string, pkgPath, name string) bool {
	s, ok := fun.(*ast.SelectorExpr)
	if !ok || s.Sel == nil || s.Sel.Name != name {
		return false
	}
	return isPkg(s.X, imports, pkgPath)
}

func collectFileConsts(file *ast.File, imports map[string]string) (map[string]uint64, map[string]string) {
	nums := map[string]uint64{}
	strs := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		bindValueSpecs(gen.Specs, nums, strs, nums, strs, imports)
	}
	return nums, strs
}

func bindValueSpecs(
	specs []ast.Spec,
	numDst map[string]uint64,
	strDst map[string]string,
	numEnv map[string]uint64,
	strEnv map[string]string,
	imports map[string]string,
) {
	for _, spec := range specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if name == nil || name.Name == "_" {
				continue
			}
			if i >= len(vs.Values) {
				continue
			}
			if u, ok := evalUint(vs.Values[i], numEnv, imports); ok {
				numDst[name.Name] = u
			}
			if s, ok := foldString(vs.Values[i], strEnv); ok {
				strDst[name.Name] = s
			}
		}
	}
}

func bindAssign(stmt *ast.AssignStmt, nums map[string]uint64, strs map[string]string, imports map[string]string) {
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" || i >= len(stmt.Rhs) {
			continue
		}
		if u, ok := evalUint(stmt.Rhs[i], nums, imports); ok {
			nums[ident.Name] = u
		}
		if s, ok := foldString(stmt.Rhs[i], strs); ok {
			strs[ident.Name] = s
		}
	}
}

func exprText(expr ast.Expr) string {
	var b strings.Builder
	writeExpr(&b, expr)
	return b.String()
}

func writeExpr(b *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		b.WriteString(e.Name)
	case *ast.BasicLit:
		b.WriteString(e.Value)
	case *ast.SelectorExpr:
		writeExpr(b, e.X)
		b.WriteByte('.')
		if e.Sel != nil {
			b.WriteString(e.Sel.Name)
		}
	case *ast.CallExpr:
		writeExpr(b, e.Fun)
		b.WriteByte('(')
		for i, a := range e.Args {
			if i > 0 {
				b.WriteString(",")
			}
			writeExpr(b, a)
		}
		b.WriteByte(')')
	case *ast.BinaryExpr:
		writeExpr(b, e.X)
		b.WriteString(e.Op.String())
		writeExpr(b, e.Y)
	case *ast.ParenExpr:
		b.WriteByte('(')
		writeExpr(b, e.X)
		b.WriteByte(')')
	case *ast.IndexExpr:
		writeExpr(b, e.X)
		b.WriteByte('[')
		writeExpr(b, e.Index)
		b.WriteByte(']')
	default:
		b.WriteString("_")
	}
}
