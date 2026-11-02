package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/scullxbones/armature/internal/gitoid"
)

const objectIDFix = "Use github.com/scullxbones/armature/internal/gitoid.Valid " +
	"(equals gitoid.PatternLower: lowercase 40|64 hex). Only internal/gitoid may encode " +
	"hash-length knowledge. Per-site waivers: allow.go (file+line+needle)."

// Probe OIDs are built from gitoid lengths — repoguard must not hard-code 40/64.
func oidProbe40() string { return strings.Repeat("a", gitoid.LenSHA1) }
func oidProbe64() string { return strings.Repeat("a", gitoid.LenSHA256) }

var (
	// oidLengthQuant matches {N} / {N,N} / BRE \{N\} where N is a gitoid width.
	oidLengthQuant = regexp.MustCompile(fmt.Sprintf(
		`\\?\{(%d|%d)(?:,(?:%d|%d))?\\?\}`,
		gitoid.LenSHA1, gitoid.LenSHA256, gitoid.LenSHA1, gitoid.LenSHA256,
	))
	breQuant    = regexp.MustCompile(`\\\{(\d+)\\\}`)
	quotedToken = regexp.MustCompile(
		`(?:r?'''([\s\S]*?)'''|r?"""([\s\S]*?)"""|r?'([^']*)'|r?"([^"]*)"|'([^']*)'|"([^"]*)"|` +
			"`" + `([^` + "`" + `]*)` + "`" + `)`,
	)
	awkSlashToken = regexp.MustCompile(`/(?:[^/\\]|\\.)+/`)
	hexNameHint   = regexp.MustCompile(`(?i)hex|oid|sha|object.?id|git.?id`)
)

func checkObjectID(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	var hits []string
	files, fset, err := parseGoTree(root, false)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	for _, pf := range files {
		if underDir(pf.rel, "testdata") || underDir(pf.rel, "scripts/repoguard") || underDir(pf.rel, "internal/gitoid") {
			continue
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					s, err := strconv.Unquote(x.Value)
					if err != nil {
						return true
					}
					pos := fset.Position(x.Pos())
					for _, reason := range oidStringReasons(s) {
						if allowedObjectIDSite(pf.rel, pos.Line, reason.needle) {
							continue
						}
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason.msg))
					}
				}
			case *ast.BinaryExpr:
				if reason := oidLenCompareReason(x); reason != "" {
					pos := fset.Position(x.Pos())
					if !allowedObjectIDSite(pf.rel, pos.Line, "len-compare-40") {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason))
					}
				}
			case *ast.ValueSpec:
				for _, reason := range oidHexLenConstReasons(x) {
					pos := fset.Position(x.Pos())
					if !allowedObjectIDSite(pf.rel, pos.Line, reason.needle) {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason.msg))
					}
				}
			}
			return true
		})
	}
	schemaHits, err := checkSchemaObjectIDPatterns(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	hits = append(hits, schemaHits...)
	textHits, err := checkTextObjectIDPatterns(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	hits = append(hits, textHits...)
	if len(hits) > 0 {
		for _, h := range hits {
			fmt.Fprintln(stderr, "FAIL: "+h)
		}
		fmt.Fprintln(stderr, objectIDFix)
		return fmt.Errorf("object-id hex-length knowledge outside gitoid")
	}
	fmt.Fprintln(stdout, "OK: object-id patterns accept SHA-1 (40) and SHA-256 (64) via gitoid")
	return nil
}

type oidHitReason struct {
	msg    string
	needle string
}

func oidStringReasons(s string) []oidHitReason {
	var out []oidHitReason
	if isExactHexWidth(s, gitoid.LenSHA1) {
		out = append(out, oidHitReason{
			msg:    fmt.Sprintf("%d-hex BasicLit encodes SHA-1 width; use gitoid.LenSHA1 / gitoid.Valid", gitoid.LenSHA1),
			needle: s,
		})
	}
	// Dual-width wholes (gitoid.PatternLower) must not be condemned for containing
	// a {40} alternative — only 40-only knowledge is banned.
	if oidPatternAcceptsBoth(s) {
		return out
	}
	for _, cand := range oidPatternCandidates(s) {
		if !oidPatternIsBanned(cand) {
			continue
		}
		out = append(out, oidHitReason{
			msg:    "pattern encodes 40-hex-only object-id width; use gitoid.Valid (40|64)",
			needle: cand,
		})
	}
	return out
}

func oidPatternAcceptsBoth(s string) bool {
	re, err := regexp.Compile(normalizeBRE(s))
	if err != nil {
		return false
	}
	// MatchString is unanchored in Go; whole-input means FindString equals the OID.
	p40, p64 := oidProbe40(), oidProbe64()
	return re.FindString(p40) == p40 && re.FindString(p64) == p64
}

func isExactHexWidth(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// oidPatternCandidates yields the whole string plus subpatterns that quantify a
// gitoid hash width. Subpattern extraction is length-driven from gitoid — not a
// growing denylist of character-class spellings.
func oidPatternCandidates(s string) []string {
	norm := normalizeBRE(s)
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(norm)
	for _, m := range oidLengthQuant.FindAllStringIndex(norm, -1) {
		add(expandOIDAtom(norm, m[0], m[1]))
	}
	return out
}

func expandOIDAtom(s string, quantStart, quantEnd int) string {
	i := quantStart
	for i > 0 {
		prev := s[i-1]
		if prev == ']' {
			depth := 1
			i--
			for i > 0 && depth > 0 {
				i--
				switch s[i] {
				case ']':
					depth++
				case '[':
					depth--
				}
			}
			continue
		}
		if prev == 'w' || prev == 'd' || prev == 's' {
			if i >= 2 && s[i-2] == '\\' {
				i -= 2
				continue
			}
		}
		if prev == ')' {
			depth := 1
			i--
			for i > 0 && depth > 0 {
				i--
				switch s[i] {
				case ')':
					depth++
				case '(':
					depth--
				}
			}
			continue
		}
		break
	}
	start := i
	for start > 0 {
		r := rune(s[start-1])
		if r == '^' || r == ':' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '\\' || r == '-' || r == '_' || r == '.' || r == '[' || r == ']' {
			start--
			continue
		}
		break
	}
	end := quantEnd
	if end < len(s) && s[end] == '$' {
		end++
	}
	return s[start:end]
}

// oidPatternIsBanned is the runtime class probe over patterns that quantify a
// gitoid hash width. Compile error → hit (fail-closed). Whole-input probe:
// FindString(40hex)==40hex && FindString(64hex)!=64hex → hit (covers anchored
// 40-only and unanchored {40} that only carves a 40-prefix of 64-hex).
func oidPatternIsBanned(s string) bool {
	norm := normalizeBRE(s)
	if !oidLengthQuant.MatchString(norm) {
		return false
	}
	re, err := regexp.Compile(norm)
	if err != nil {
		return true
	}
	p40, p64 := oidProbe40(), oidProbe64()
	return re.FindString(p40) == p40 && re.FindString(p64) != p64
}

func normalizeBRE(s string) string {
	return breQuant.ReplaceAllString(s, `{$1}`)
}

// oidLenCompareReason flags len(x) compared to gitoid.LenSHA1 with any relational op.
func oidLenCompareReason(bin *ast.BinaryExpr) string {
	switch bin.Op {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
	default:
		return ""
	}
	if litIsHashLen(bin.Y) && isLenCall(bin.X) {
		return "len(x) compared to SHA-1 width; use gitoid.Valid (40|64)"
	}
	if litIsHashLen(bin.X) && isLenCall(bin.Y) {
		return "len(x) compared to SHA-1 width; use gitoid.Valid (40|64)"
	}
	return ""
}

func litIsHashLen(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == strconv.Itoa(gitoid.LenSHA1)
}

func isLenCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
}

func oidHexLenConstReasons(spec *ast.ValueSpec) []oidHitReason {
	var out []oidHitReason
	for i, name := range spec.Names {
		if name == nil || !hexNameHint.MatchString(name.Name) {
			continue
		}
		if i >= len(spec.Values) {
			continue
		}
		lit, ok := spec.Values[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			continue
		}
		if lit.Value != strconv.Itoa(gitoid.LenSHA1) {
			continue
		}
		out = append(out, oidHitReason{
			msg:    "const " + name.Name + " = " + lit.Value + " encodes SHA-1 width near hex/OID name; use gitoid.LenSHA1",
			needle: name.Name,
		})
	}
	return out
}

func checkSchemaObjectIDPatterns(root string) ([]string, error) {
	dir := filepath.Join(root, "docs", "schemas")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("docs", "schemas", e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		walkSchemaPatterns(doc, rel, "", &hits)
	}
	return hits, nil
}

func walkSchemaPatterns(node any, rel, path string, hits *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if pat, ok := v["pattern"].(string); ok {
			if oidPatternAcceptsBoth(pat) {
				// dual-width schema pattern — OK
			} else {
				for _, cand := range oidPatternCandidates(pat) {
					if !oidPatternIsBanned(cand) {
						continue
					}
					*hits = append(*hits, fmt.Sprintf("%s: pattern %s encodes 40-hex-only width; use gitoid.PatternLower", rel, path))
				}
			}
		}
		for k, child := range v {
			childPath := path
			if childPath == "" {
				childPath = k
			} else {
				childPath = path + "." + k
			}
			walkSchemaPatterns(child, rel, childPath, hits)
		}
	case []any:
		for i, child := range v {
			walkSchemaPatterns(child, rel, fmt.Sprintf("%s[%d]", path, i), hits)
		}
	}
}

func checkTextObjectIDPatterns(root string) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			switch name {
			case "testdata", "vendor", ".git", "node_modules", "ops":
				return filepath.SkipDir
			case ".github":
				return nil
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if underDir(rel, "scripts/repoguard") || underDir(rel, "docs") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".sh", ".bash", ".py", ".yml", ".yaml":
		default:
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			lineNo := i + 1
			for _, tok := range textRegexTokens(line) {
				if allowedObjectIDSite(rel, lineNo, tok) || oidPatternAcceptsBoth(tok) {
					continue
				}
				for _, cand := range oidPatternCandidates(tok) {
					if !oidPatternIsBanned(cand) {
						continue
					}
					if allowedObjectIDSite(rel, lineNo, cand) {
						continue
					}
					hits = append(hits, fmt.Sprintf("%s:%d: pattern encodes 40-hex-only object-id width; use gitoid", rel, lineNo))
				}
			}
		}
		return nil
	})
	return hits, err
}

// textRegexTokens extracts every quoted string and regex-looking token on a line.
// No command-name prefilter — grep -qE, awk //, bare assignments, YAML run: all count.
func textRegexTokens(line string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, m := range quotedToken.FindAllStringSubmatch(line, -1) {
		for _, g := range m[1:] {
			if g != "" {
				add(g)
			}
		}
	}
	for _, m := range awkSlashToken.FindAllString(line, -1) {
		if len(m) >= 2 {
			add(m[1 : len(m)-1])
		}
	}
	if idx := strings.Index(line, "=~"); idx >= 0 {
		rhs := strings.TrimSpace(line[idx+2:])
		rhs = strings.TrimSuffix(rhs, "]]")
		rhs = strings.TrimSpace(rhs)
		// Strip trailing ]] stuff already; take first token if unquoted.
		if rhs != "" && !strings.HasPrefix(rhs, "'") && !strings.HasPrefix(rhs, "\"") {
			fields := strings.Fields(rhs)
			if len(fields) > 0 {
				add(fields[0])
			}
		}
	}
	return out
}

// Coverage re-trigger marker (fix 419).
