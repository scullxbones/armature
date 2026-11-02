package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"strings"
)

// checkGitNetwork fails when production Go builds a git child whose subcommand
// is fetch, push, ls-remote, pull, or clone outside Client.networkCmd.
// Variable-held verbs, append-built argv, and aliases of the builder count.
// Local bundle and bare-path sites pass only with a matching per-site allow.
func checkGitNetwork(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	files, fset, err := parseProdGo(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}

	byDir := map[string][]parsedFile{}
	for _, pf := range files {
		dir := dirOf(pf.rel)
		byDir[dir] = append(byDir[dir], pf)
	}
	forwarders := map[string]map[string]builderRef{}
	for dir, pfs := range byDir {
		forwarders[dir] = discoverForwarders(pfs)
	}

	allows := newAllowState()
	var hits []string
	for _, pf := range files {
		cmap := ast.NewCommentMap(fset, pf.file, pf.file.Comments)
		fileStrs, fileSlices := fileFacts(pf.file)
		dir := dirOf(pf.rel)
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Body == nil {
				continue
			}
			allows.see(pf.rel, fn.Name.Name)
			env := newFactEnv(fileStrs, fileSlices)
			walkBlock(fn.Body, env, walkCtx{
				fn:         fn.Name.Name,
				rel:        pf.rel,
				imports:    pf.imports,
				forwarders: forwarders[dir],
				commentOf:  nil,
				cmap:       cmap,
				fset:       fset,
				allows:     allows,
				hits:       &hits,
			})
		}
	}

	failed := false
	if len(hits) > 0 {
		fmt.Fprintln(stderr, "FAIL: remote-network git outside networkCmd:")
		for _, h := range hits {
			fmt.Fprintln(stderr, h)
		}
		fmt.Fprintln(stderr, gitNetworkFix)
		failed = true
	}
	for _, msg := range allows.problems() {
		fmt.Fprintln(stderr, "FAIL: "+msg)
		fmt.Fprintln(stderr, gitNetworkFix)
		failed = true
	}
	if failed {
		return fmt.Errorf("git-network")
	}
	fmt.Fprintln(stdout, "OK: remote-network git calls go through networkCmd")
	for _, a := range gitNetAllows() {
		fmt.Fprintf(stdout, "  %s %s %s: %s\n", a.RelPath, a.Func, a.Site, a.Reason)
	}
	return nil
}

const gitNetworkFix = "Use networkCmd so the call gets the network deadline, process-group cancel, and named-remote transport. " +
	"Local bundle files and local bare paths need a per-site gitnetallow, not a file-wide exemption."

func dirOf(rel string) string {
	rel = strings.TrimSuffix(rel, "/")
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return "."
	}
	return rel[:i]
}

type builderKind int

const (
	kindArgv builderKind = iota
	kindExec
	kindSanctioned
)

type builderRef struct {
	kind     builderKind
	argvFrom int
	nameFrom int
}

type elem struct {
	known bool
	val   string
}

type factEnv struct {
	strs    map[string]string
	slices  map[string][]elem
	aliases map[string]builderRef
}

func newFactEnv(strs map[string]string, slices map[string][]elem) *factEnv {
	e := &factEnv{
		strs:    map[string]string{},
		slices:  map[string][]elem{},
		aliases: map[string]builderRef{},
	}
	for k, v := range strs {
		e.strs[k] = v
	}
	for k, v := range slices {
		e.slices[k] = append([]elem(nil), v...)
	}
	return e
}

func (e *factEnv) clone() *factEnv {
	out := newFactEnv(e.strs, e.slices)
	for k, v := range e.aliases {
		out.aliases[k] = v
	}
	return out
}

type walkCtx struct {
	fn         string
	rel        string
	imports    map[string]string
	forwarders map[string]builderRef
	commentOf  ast.Node
	cmap       ast.CommentMap
	fset       *token.FileSet
	allows     *allowState
	hits       *[]string
}

func walkBlock(block *ast.BlockStmt, env *factEnv, ctx walkCtx) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		walkStmt(stmt, env, ctx)
	}
}

func walkStmt(stmt ast.Stmt, env *factEnv, ctx walkCtx) {
	if stmt == nil {
		return
	}
	commentNode := ctx.commentOf
	if commentNode == nil {
		commentNode = stmt
	}
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		walkBlock(s, env.clone(), ctx)
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		bindFacts(gen, env, ctx.imports)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			scanExpr(rhs, env, commentNode, ctx)
		}
		bindAssignFacts(s, env, ctx.imports)
	case *ast.ExprStmt:
		scanExpr(s.X, env, commentNode, ctx)
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			scanExpr(r, env, commentNode, ctx)
		}
	case *ast.GoStmt:
		scanExpr(s.Call, env, commentNode, ctx)
	case *ast.DeferStmt:
		scanExpr(s.Call, env, commentNode, ctx)
	case *ast.IfStmt:
		if s.Init != nil {
			saved := ctx.commentOf
			ctx.commentOf = s
			walkStmt(s.Init, env, ctx)
			ctx.commentOf = saved
		}
		scanExpr(s.Cond, env, s, ctx)
		walkBlock(s.Body, env.clone(), ctx)
		if s.Else != nil {
			walkStmt(s.Else, env.clone(), ctx)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			walkStmt(s.Init, env, ctx)
		}
		scanExpr(s.Cond, env, s, ctx)
		walkStmt(s.Post, env.clone(), ctx)
		walkBlock(s.Body, env.clone(), ctx)
	case *ast.RangeStmt:
		scanExpr(s.X, env, s, ctx)
		walkBlock(s.Body, env.clone(), ctx)
	case *ast.SwitchStmt:
		if s.Init != nil {
			walkStmt(s.Init, env.clone(), ctx)
		}
		scanExpr(s.Tag, env, s, ctx)
		walkBlock(s.Body, env, ctx)
	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			walkStmt(s.Init, env.clone(), ctx)
		}
		scanStmt(s.Assign, env, s, ctx)
		walkBlock(s.Body, env, ctx)
	case *ast.CaseClause:
		for _, e := range s.List {
			scanExpr(e, env, s, ctx)
		}
		for _, st := range s.Body {
			walkStmt(st, env, ctx)
		}
	case *ast.LabeledStmt:
		walkStmt(s.Stmt, env, ctx)
	case *ast.SendStmt:
		scanExpr(s.Chan, env, commentNode, ctx)
		scanExpr(s.Value, env, commentNode, ctx)
	case *ast.IncDecStmt:
		scanExpr(s.X, env, commentNode, ctx)
	default:
		scanStmt(stmt, env, commentNode, ctx)
	}
}

func scanStmt(stmt ast.Stmt, env *factEnv, comment ast.Node, ctx walkCtx) {
	if stmt == nil {
		return
	}
	ast.Inspect(stmt, func(n ast.Node) bool {
		if n == nil || n == stmt {
			return true
		}
		if _, ok := n.(*ast.BlockStmt); ok {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok {
			considerCall(call, env, comment, ctx)
		}
		return true
	})
}

func scanExpr(e ast.Expr, env *factEnv, comment ast.Node, ctx walkCtx) {
	if e == nil {
		return
	}
	ast.Inspect(e, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok {
			considerCall(call, env, comment, ctx)
		}
		return true
	})
}

func considerCall(call *ast.CallExpr, env *factEnv, comment ast.Node, ctx walkCtx) {
	b, ok := resolveBuilder(call.Fun, env, ctx)
	if !ok || b.kind == kindSanctioned {
		return
	}
	if b.kind == kindExec {
		if !execNameIsGit(call, b, env) {
			return
		}
	}
	if b.argvFrom > len(call.Args) {
		return
	}
	elems := evalElems(call.Args[b.argvFrom:], call.Ellipsis.IsValid(), env)
	verb, known := networkSubcommand(elems)
	if !known || !networkVerbs()[verb] {
		return
	}
	if ctx.allows.permit(ctx.rel, ctx.fn, commentText(ctx.cmap, comment)) {
		return
	}
	*ctx.hits = append(*ctx.hits, formatHit(ctx.fset, call.Pos(), ctx.rel, "remote-network git "+verb+" outside networkCmd"))
}

func execNameIsGit(call *ast.CallExpr, b builderRef, env *factEnv) bool {
	if b.nameFrom < 0 || b.nameFrom >= len(call.Args) {
		return false
	}
	name, ok := foldString(call.Args[b.nameFrom], env.strs)
	return ok && name == "git"
}

func resolveBuilder(fun ast.Expr, env *factEnv, ctx walkCtx) (builderRef, bool) {
	switch f := fun.(type) {
	case *ast.Ident:
		if b, ok := env.aliases[f.Name]; ok {
			return b, true
		}
		if b, ok := ctx.forwarders[f.Name]; ok {
			return b, true
		}
		return identBuilder(f.Name)
	case *ast.SelectorExpr:
		if f.Sel == nil {
			return builderRef{}, false
		}
		if b, ok := selectorBuilder(f, ctx.imports); ok {
			return b, true
		}
		if b, ok := ctx.forwarders[f.Sel.Name]; ok {
			return b, true
		}
		return identBuilder(f.Sel.Name)
	case *ast.ParenExpr:
		return resolveBuilder(f.X, env, ctx)
	default:
		return builderRef{}, false
	}
}

func identBuilder(name string) (builderRef, bool) {
	switch name {
	case "networkCmd":
		return builderRef{kind: kindSanctioned, argvFrom: 0}, true
	case "cmd":
		return builderRef{kind: kindArgv, argvFrom: 0}, true
	case "cmdContext":
		return builderRef{kind: kindArgv, argvFrom: 1}, true
	case "CommandGit":
		return builderRef{kind: kindArgv, argvFrom: 2}, true
	case "NonInteractiveGitCommand", "gitCommand":
		return builderRef{kind: kindArgv, argvFrom: 1}, true
	default:
		return builderRef{}, false
	}
}

func selectorBuilder(sel *ast.SelectorExpr, imports map[string]string) (builderRef, bool) {
	if sel.Sel == nil {
		return builderRef{}, false
	}
	switch sel.Sel.Name {
	case "Command":
		if isPkg(sel.X, imports, "os/exec") {
			return builderRef{kind: kindExec, nameFrom: 0, argvFrom: 1}, true
		}
	case "CommandContext":
		if isPkg(sel.X, imports, "os/exec") {
			return builderRef{kind: kindExec, nameFrom: 1, argvFrom: 2}, true
		}
	}
	return identBuilder(sel.Sel.Name)
}

func networkVerbs() map[string]bool {
	return map[string]bool{
		"fetch": true, "push": true, "ls-remote": true, "pull": true, "clone": true,
	}
}

func networkSubcommand(elems []elem) (string, bool) {
	for i := 0; i < len(elems); i++ {
		el := elems[i]
		if !el.known {
			return "", false
		}
		if strings.HasPrefix(el.val, "-") {
			if strings.Contains(el.val, "=") {
				continue
			}
			if gitOptTakesValue(el.val) {
				if i+1 >= len(elems) {
					return "", false
				}
				i++
			}
			continue
		}
		return el.val, true
	}
	return "", false
}

func gitOptTakesValue(opt string) bool {
	switch opt {
	case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--super-prefix", "--config-env", "--exec-path":
		return true
	default:
		return false
	}
}

func evalElems(args []ast.Expr, ellipsis bool, env *factEnv) []elem {
	if len(args) == 0 {
		return nil
	}
	if ellipsis && len(args) == 1 {
		if sv, ok := evalSlice(args[0], env); ok {
			return sv
		}
		return []elem{{known: false}}
	}
	var out []elem
	for i, a := range args {
		spread := ellipsis && i == len(args)-1
		if spread {
			if sv, ok := evalSlice(a, env); ok {
				out = append(out, sv...)
			} else {
				out = append(out, elem{known: false})
			}
			continue
		}
		if s, ok := foldString(a, env.strs); ok {
			out = append(out, elem{known: true, val: s})
		} else {
			out = append(out, elem{known: false})
		}
	}
	return out
}

func evalSlice(expr ast.Expr, env *factEnv) ([]elem, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		sv, ok := env.slices[e.Name]
		if !ok {
			return nil, false
		}
		return append([]elem(nil), sv...), true
	case *ast.ParenExpr:
		return evalSlice(e.X, env)
	case *ast.CompositeLit:
		var out []elem
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			if s, ok := foldString(elt, env.strs); ok {
				out = append(out, elem{known: true, val: s})
			} else {
				out = append(out, elem{known: false})
			}
		}
		return out, true
	case *ast.CallExpr:
		if !isAppend(e.Fun) || len(e.Args) == 0 {
			return nil, false
		}
		base, ok := evalSlice(e.Args[0], env)
		if !ok {
			if s, sok := foldString(e.Args[0], env.strs); sok {
				base = []elem{{known: true, val: s}}
			} else {
				base = []elem{{known: false}}
			}
		}
		for i, a := range e.Args[1:] {
			spread := e.Ellipsis.IsValid() && i == len(e.Args[1:])-1
			if spread {
				if sv, sok := evalSlice(a, env); sok {
					base = append(base, sv...)
				} else {
					base = append(base, elem{known: false})
				}
				continue
			}
			if s, sok := foldString(a, env.strs); sok {
				base = append(base, elem{known: true, val: s})
			} else {
				base = append(base, elem{known: false})
			}
		}
		return base, true
	default:
		return nil, false
	}
}

func isAppend(fun ast.Expr) bool {
	id, ok := fun.(*ast.Ident)
	return ok && id.Name == "append"
}

func bindFacts(gen *ast.GenDecl, env *factEnv, imports map[string]string) {
	if gen.Tok != token.CONST && gen.Tok != token.VAR {
		return
	}
	nums := map[string]uint64{}
	bindValueSpecs(gen.Specs, nums, env.strs, nums, env.strs, imports)
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if name == nil || name.Name == "_" || i >= len(vs.Values) {
				continue
			}
			bindOne(name.Name, vs.Values[i], env, imports)
		}
	}
}

func bindAssignFacts(stmt *ast.AssignStmt, env *factEnv, imports map[string]string) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	bindAssign(stmt, map[string]uint64{}, env.strs, imports)
	for i, lhs := range stmt.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		bindOne(id.Name, stmt.Rhs[i], env, imports)
	}
}

func bindOne(name string, expr ast.Expr, env *factEnv, imports map[string]string) {
	if sv, ok := evalSlice(expr, env); ok {
		env.slices[name] = sv
	} else {
		delete(env.slices, name)
	}
	if _, ok := foldString(expr, env.strs); !ok {
		delete(env.strs, name)
	}
	if b, ok := evalAlias(expr, env, imports); ok {
		env.aliases[name] = b
	} else {
		delete(env.aliases, name)
	}
}

func evalAlias(expr ast.Expr, env *factEnv, imports map[string]string) (builderRef, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		if b, ok := env.aliases[e.Name]; ok {
			return b, true
		}
		return identBuilder(e.Name)
	case *ast.SelectorExpr:
		return selectorBuilder(e, imports)
	case *ast.ParenExpr:
		return evalAlias(e.X, env, imports)
	default:
		return builderRef{}, false
	}
}

func fileFacts(file *ast.File) (map[string]string, map[string][]elem) {
	_, strs := collectFileConsts(file, importMap(file))
	env := newFactEnv(strs, nil)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		bindFacts(gen, env, importMap(file))
	}
	return env.strs, env.slices
}

type fnFact struct {
	fn      *ast.FuncDecl
	imports map[string]string
}

func discoverForwarders(files []parsedFile) map[string]builderRef {
	counts := map[string]int{}
	var funcs []fnFact
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Body == nil {
				continue
			}
			counts[fn.Name.Name]++
			funcs = append(funcs, fnFact{fn: fn, imports: pf.imports})
		}
	}
	out := map[string]builderRef{}
	for {
		changed := false
		for _, item := range funcs {
			name := item.fn.Name.Name
			if counts[name] != 1 || name == "networkCmd" {
				continue
			}
			if _, ok := out[name]; ok {
				continue
			}
			param, index, ok := variadicStringParam(item.fn)
			if !ok {
				continue
			}
			if forwardsArgv(item.fn.Body, param, item.imports, out) {
				out[name] = builderRef{kind: kindArgv, argvFrom: index}
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return out
}

func variadicStringParam(fn *ast.FuncDecl) (string, int, bool) {
	if fn.Type == nil || fn.Type.Params == nil {
		return "", 0, false
	}
	index := 0
	for _, field := range fn.Type.Params.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		if _, ok := field.Type.(*ast.Ellipsis); ok && len(field.Names) == 1 {
			return field.Names[0].Name, index, true
		}
		index += n
	}
	return "", 0, false
}

func forwardsArgv(body *ast.BlockStmt, param string, imports map[string]string, known map[string]builderRef) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || !call.Ellipsis.IsValid() {
			return true
		}
		last, ok := call.Args[len(call.Args)-1].(*ast.Ident)
		if !ok || last.Name != param {
			return true
		}
		b, ok := callBuilder(call.Fun, imports, known)
		if !ok || b.kind == kindSanctioned {
			return true
		}
		if b.kind == kindExec && !execNameIsGit(call, b, newFactEnv(map[string]string{}, nil)) {
			return true
		}
		if len(call.Args)-1 == b.argvFrom {
			found = true
		}
		return true
	})
	return found
}

func callBuilder(fun ast.Expr, imports map[string]string, known map[string]builderRef) (builderRef, bool) {
	switch f := fun.(type) {
	case *ast.Ident:
		if b, ok := known[f.Name]; ok {
			return b, true
		}
		return identBuilder(f.Name)
	case *ast.SelectorExpr:
		if b, ok := selectorBuilder(f, imports); ok {
			return b, true
		}
		if f.Sel != nil {
			if b, ok := known[f.Sel.Name]; ok {
				return b, true
			}
			return identBuilder(f.Sel.Name)
		}
	case *ast.ParenExpr:
		return callBuilder(f.X, imports, known)
	}
	return builderRef{}, false
}

type allowUse struct {
	entry gitNetAllow
	seen  bool
	used  int
}

type allowState struct {
	uses []allowUse
}

func newAllowState() *allowState {
	entries := gitNetAllows()
	st := &allowState{uses: make([]allowUse, len(entries))}
	for i, e := range entries {
		st.uses[i].entry = e
	}
	return st
}

func (s *allowState) see(rel, fn string) {
	for i := range s.uses {
		if s.uses[i].entry.RelPath == rel && s.uses[i].entry.Func == fn {
			s.uses[i].seen = true
		}
	}
}

func (s *allowState) permit(rel, fn, comment string) bool {
	site, reason, ok := parseGitNetAllow(comment)
	if !ok {
		return false
	}
	for i := range s.uses {
		e := s.uses[i].entry
		if e.RelPath != rel || e.Func != fn || e.Site != site {
			continue
		}
		if !strings.Contains(reason, e.Reason) && !strings.Contains(comment, e.Reason) {
			continue
		}
		s.uses[i].used++
		return s.uses[i].used == 1
	}
	return false
}

func (s *allowState) problems() []string {
	var out []string
	for _, u := range s.uses {
		if !u.seen {
			continue
		}
		if u.used != 1 {
			out = append(out, fmt.Sprintf(
				"git-network allow %s %s %s used %d time(s); want one per-site gitnetallow",
				u.entry.RelPath, u.entry.Func, u.entry.Site, u.used,
			))
		}
	}
	return out
}

func parseGitNetAllow(comment string) (string, string, bool) {
	const marker = "gitnetallow:"
	i := strings.Index(comment, marker)
	if i < 0 {
		return "", "", false
	}
	rest := strings.TrimSpace(comment[i+len(marker):])
	site, reason, ok := strings.Cut(rest, " ")
	if !ok {
		return "", "", false
	}
	site = strings.TrimSpace(site)
	reason = strings.TrimSpace(reason)
	if site == "" || reason == "" || strings.Contains(site, "\n") {
		return "", "", false
	}
	return site, reason, true
}

func commentText(cmap ast.CommentMap, n ast.Node) string {
	if n == nil || cmap == nil {
		return ""
	}
	var b strings.Builder
	for _, g := range cmap[n] {
		for _, c := range g.List {
			b.WriteString(c.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
