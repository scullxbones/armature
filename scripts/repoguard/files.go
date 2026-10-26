package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type parsedFile struct {
	rel     string
	path    string
	pkg     string
	file    *ast.File
	imports map[string]string
}

func requireGoMod(root string) error {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("go.mod not found in %s", root)
	}
	return nil
}

func parseGoTree(root string, testOnly bool) ([]parsedFile, *token.FileSet, error) {
	fset := token.NewFileSet()
	var out []parsedFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if name == "testdata" || name == "vendor" || name == ".git" || name == "node_modules" || strings.HasPrefix(name, ".") {
				if path == root {
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if testOnly && !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isIgnoreTagged(src) {
			return nil
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			return fmt.Errorf("parse %s: %w", filepath.ToSlash(rel), err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		pkg := ""
		if file.Name != nil {
			pkg = file.Name.Name
		}
		out = append(out, parsedFile{
			rel:     rel,
			path:    path,
			pkg:     pkg,
			file:    file,
			imports: importMap(file),
		})
		return nil
	})
	return out, fset, err
}

func importMap(file *ast.File) map[string]string {
	m := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconvUnquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
			if name == "." || name == "_" {
				continue
			}
		} else {
			name = pathBase(path)
		}
		m[name] = path
	}
	return m
}

func pathBase(importPath string) string {
	if i := strings.LastIndex(importPath, "/"); i >= 0 {
		return importPath[i+1:]
	}
	return importPath
}

func isIgnoreTagged(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "//go:build") {
			return strings.Contains(trim, "ignore")
		}
		if strings.HasPrefix(trim, "// +build") {
			return strings.Contains(trim, "ignore")
		}
		if strings.HasPrefix(trim, "//") {
			continue
		}
		return false
	}
	return false
}

func underDir(rel, dir string) bool {
	rel = filepath.ToSlash(rel)
	dir = strings.TrimSuffix(filepath.ToSlash(dir), "/")
	return rel == dir || strings.HasPrefix(rel, dir+"/")
}

func pkgKey(rel, pkg string) string {
	return filepath.ToSlash(filepath.Dir(rel)) + " " + pkg
}
