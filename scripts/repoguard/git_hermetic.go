package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"regexp"
	"sort"
)

var (
	gitInitWord   = regexp.MustCompile(`\bgit(?:\s+-[Cc]\s+\S+)+\s+init\b`)
	gitInitSimple = regexp.MustCompile(`\bgit\s+init\b`)
)

func gitInitFlags() map[string]bool {
	return map[string]bool{"--bare": true, "-b": true, "-q": true}
}

func gittestHelpers() map[string]bool {
	return map[string]bool{
		"Init": true, "InitRepo": true, "InitWithOrigin": true, "Git": true, "Main": true, "IsolateGit": true,
	}
}

func gitIdentHelpers() map[string]bool {
	return map[string]bool{
		"GitInitMain": true, "GitInitBareMain": true, "NonInteractiveGitCommand": true, "IsolateGit": true,
	}
}

func checkGitHermetic(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	files, fset, err := parseGoTree(root, true)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}

	var initHits []string
	gitPkgs := map[string]bool{}
	isoPkgs := map[string]bool{}

	for _, pf := range files {
		fileNums, fileStrs := collectFileConsts(pf.file, pf.imports)
		inGittest := underDir(pf.rel, "internal/gittest")
		nameAllow := allowed(pf.rel, allowGitHermeticName)
		initAllow := allowed(pf.rel, allowGitHermeticInit)
		usesGit := false
		isolated := false

		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Name != nil && x.Name.Name == "TestMain" && testMainIsolates(x) {
					isolated = true
				}
			case *ast.GenDecl:
				if x.Tok == token.CONST || x.Tok == token.VAR {
					bindValueSpecs(x.Specs, fileNums, fileStrs, fileNums, fileStrs, pf.imports)
				}
			case *ast.AssignStmt:
				bindAssign(x, fileNums, fileStrs, pf.imports)
			case *ast.CallExpr:
				if gitCallExec(x, pf.imports, fileStrs) {
					usesGit = true
				}
				if !inGittest && !initAllow && gitCallInit(x, pf.imports, fileStrs) {
					initHits = append(initHits, formatHit(fset, x.Pos(), pf.rel, "raw git init"))
				}
			case *ast.CompositeLit:
				args := foldedElems(x, fileStrs)
				if gitArgv(args) {
					usesGit = true
				}
				if !inGittest && !initAllow && gitArgvInit(args) {
					initHits = append(initHits, formatHit(fset, x.Pos(), pf.rel, "raw git init"))
				}
			case *ast.BasicLit:
				if s, ok := foldString(x, fileStrs); ok {
					if gitInitInText(s) && !inGittest && !initAllow {
						initHits = append(initHits, formatHit(fset, x.Pos(), pf.rel, "raw git init"))
					}
					if s == "git" && !nameAllow {
						usesGit = true
					}
				}
			case *ast.BinaryExpr:
				if s, ok := foldString(x, fileStrs); ok {
					if gitInitInText(s) && !inGittest && !initAllow {
						initHits = append(initHits, formatHit(fset, x.Pos(), pf.rel, "raw git init"))
					}
					if s == "git" && !nameAllow {
						usesGit = true
					}
				}
			case *ast.Ident:
				if gitIdentHelpers()[x.Name] {
					usesGit = true
				}
			case *ast.SelectorExpr:
				if x.Sel != nil && isPkg(x.X, pf.imports, "github.com/scullxbones/armature/internal/gittest") && gittestHelpers()[x.Sel.Name] {
					usesGit = true
				}
				if x.Sel != nil && gitIdentHelpers()[x.Sel.Name] {
					usesGit = true
				}
			}
			return true
		})

		key := pkgKey(pf.rel, pf.pkg)
		if usesGit {
			gitPkgs[key] = true
		}
		if isolated {
			isoPkgs[key] = true
		}
	}

	failed := false
	if len(initHits) > 0 {
		fmt.Fprintln(stderr, "FAIL: raw git init in test files outside internal/gittest:")
		for _, h := range initHits {
			fmt.Fprintln(stderr, h)
		}
		fmt.Fprintln(stderr, "Use gittest.Init / gittest.InitRepo instead.")
		failed = true
	} else {
		fmt.Fprintln(stdout, "OK: no raw git init in _test.go files outside internal/gittest")
	}

	var missing []string
	for key := range gitPkgs {
		if !isoPkgs[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		fmt.Fprintln(stderr, "FAIL: packages exec git in tests without TestMain isolation (gittest.IsolateGit / gittest.Main):")
		for _, key := range missing {
			fmt.Fprintln(stderr, key)
		}
		fmt.Fprintln(stderr, "Add a TestMain that calls gittest.Main or gittest.IsolateGit.")
		failed = true
	} else {
		fmt.Fprintln(stdout, "OK: every git-executing test package has TestMain isolation")
	}

	fmt.Fprintln(stdout, "OK: git-hermetic allows:")
	for _, a := range allowlist() {
		if a.Check != allowGitHermeticName && a.Check != allowGitHermeticInit {
			continue
		}
		fmt.Fprintf(stdout, "  %s (%s): %s\n", a.RelPath, a.Check, a.Reason)
	}

	if failed {
		return fmt.Errorf("git-test-hermetic")
	}
	return nil
}

func gitInitInText(s string) bool {
	return gitInitWord.MatchString(s) || gitInitSimple.MatchString(s)
}

func foldedElems(lit *ast.CompositeLit, env map[string]string) []string {
	var out []string
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		if s, ok := foldString(elt, env); ok {
			out = append(out, s)
		} else {
			out = append(out, "")
		}
	}
	return out
}

func foldedArgs(call *ast.CallExpr, env map[string]string) []string {
	var out []string
	for _, a := range call.Args {
		if s, ok := foldString(a, env); ok {
			out = append(out, s)
		} else {
			out = append(out, "")
		}
	}
	return out
}

func gitArgv(args []string) bool {
	for _, a := range args {
		if a == "git" {
			return true
		}
	}
	return false
}

func gitArgvInit(args []string) bool {
	if looksLikeGitInitArgs(args) {
		return true
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "init" && gitInitFlags()[args[i+1]] {
			return true
		}
	}
	return false
}

func looksLikeGitInitArgs(args []string) bool {
	i := 0
	if i < len(args) && args[i] == "git" {
		i++
	} else {
		return false
	}
	for i+1 < len(args) && (args[i] == "-C" || args[i] == "-c") {
		i += 2
	}
	return i < len(args) && args[i] == "init"
}

func gitCallExec(call *ast.CallExpr, imports map[string]string, env map[string]string) bool {
	args := foldedArgs(call, env)
	switch {
	case sel(call.Fun, imports, "os/exec", "Command"):
		return len(args) > 0 && args[0] == "git"
	case sel(call.Fun, imports, "os/exec", "CommandContext"):
		return len(args) > 1 && args[1] == "git"
	case sel(call.Fun, imports, "os/exec", "LookPath"):
		return len(args) > 0 && args[0] == "git"
	default:
		return false
	}
}

func gitCallInit(call *ast.CallExpr, imports map[string]string, env map[string]string) bool {
	args := foldedArgs(call, env)
	switch {
	case sel(call.Fun, imports, "os/exec", "Command"):
		return gitArgvInit(args)
	case sel(call.Fun, imports, "os/exec", "CommandContext"):
		if len(args) > 0 {
			return gitArgvInit(args[1:])
		}
	}
	return gitArgvInit(args)
}

func testMainIsolates(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "IsolateGit" || fun.Name == "Main" {
				found = true
			}
		case *ast.SelectorExpr:
			if fun.Sel != nil && (fun.Sel.Name == "IsolateGit" || fun.Sel.Name == "Main") {
				found = true
			}
		}
		return true
	})
	return found
}
