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

func oidProbe40() string { return strings.Repeat("a", gitoid.LenSHA1) }
func oidProbe64() string { return strings.Repeat("a", gitoid.LenSHA256) }

var (
	oidLengthQuant = regexp.MustCompile(fmt.Sprintf(
		`\\?\{(%d|%d)(?:,(?:%d|%d))?\\?\}`,
		gitoid.LenSHA1, gitoid.LenSHA256, gitoid.LenSHA1, gitoid.LenSHA256,
	))
	breQuant = regexp.MustCompile(`\\\{(\d+)\\\}`)
)

// textSkipDirList is the explicit, justified exclusion list for the object-id
// text walk. Anything else under the repo root is scanned (subject to extension
// filter). .github is intentionally NOT skipped so workflows are covered.
// Non-.github dot-dirs are skipped via textSkipDir (prefix rule kept here so
// the exclusion policy lives in one place).
func textSkipDirList() []string {
	return []string{
		"testdata",     // fixture corpora, not product patterns
		"vendor",       // third-party
		".git",         // git objects (also matched by dot-dir rule)
		"node_modules", // third-party
		"ops",          // runtime armature state (non-gating follow-up #427)
		"docs",         // prose/schemas walked separately for schema patterns (#427)
	}
}

// textSkipDir reports whether a directory name should be skipped during the
// object-id text walk. .github is scanned; other dot-dirs and textSkipDirList
// entries are skipped.
func textSkipDir(name string) bool {
	if name == ".github" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true // non-.github dot-dirs
	}
	for _, skip := range textSkipDirList() {
		if name == skip {
			return true
		}
	}
	return false
}

func checkObjectID(root string, stdout, stderr io.Writer) error {
	if err := validateAllowlist(); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	idx := newAllowIndex(allowlist())
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
				if x.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(x.Value)
				if err != nil {
					return true
				}
				pos := fset.Position(x.Pos())
				for _, reason := range oidStringReasons(s) {
					if idx.allowed(pf.rel, allowObjectID, pos.Line, reason.needle) {
						continue
					}
					hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason.msg))
				}
			case *ast.BinaryExpr:
				if reason := oidWidthCompareReason(x); reason != "" {
					pos := fset.Position(x.Pos())
					if !idx.allowed(pf.rel, allowObjectID, pos.Line, "width-compare") {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason))
					}
				}
			case *ast.ValueSpec:
				for _, reason := range oidWidthConstReasons(x) {
					pos := fset.Position(x.Pos())
					if !idx.allowed(pf.rel, allowObjectID, pos.Line, reason.needle) {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason.msg))
					}
				}
			}
			return true
		})
	}
	schemaHits, err := checkSchemaObjectIDPatterns(root, idx)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	hits = append(hits, schemaHits...)
	textHits, err := checkTextObjectIDPatterns(root, idx)
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
	if isArmatureRoot(root) {
		if err := idx.unused(allowObjectID); err != nil {
			fmt.Fprintf(stderr, "FAIL: %v\n", err)
			return err
		}
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
	if isExactHexWidth(s, gitoid.LenSHA1) || isExactHexWidth(s, gitoid.LenSHA256) {
		out = append(out, oidHitReason{
			msg:    fmt.Sprintf("%d-hex BasicLit encodes gitoid width; use gitoid.PadSHA1/PadSHA256 or Len*", len(s)),
			needle: s,
		})
	}
	if oidPatternAcceptsBoth(s) {
		return out
	}
	for _, cand := range oidPatternCandidates(s) {
		if reason := oidPatternBanReason(cand); reason != "" {
			out = append(out, oidHitReason{msg: reason, needle: cand})
		}
	}
	return out
}

func oidPatternAcceptsBoth(s string) bool {
	re, err := regexp.Compile(normalizeBRE(s))
	if err != nil {
		return false
	}
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

var oidHexClass = regexp.MustCompile(`(?i)\[(?:0-9a-f|a-f0-9)[^\]]*\]|\[\[:xdigit:\]\]|\\x\[0-9a-f`)

// oidPatternBanReason: fail-closed. Compile error on an OID-alphabet candidate
// → banned (plant: ([0-9a-f]+). Single-width (only-40 or only-64) → banned.
// Dual-width → empty (ok).
func oidPatternBanReason(s string) string {
	norm := normalizeBRE(s)
	re, err := regexp.Compile(norm)
	if err != nil {
		if oidHexClass.MatchString(norm) || oidLengthQuant.MatchString(norm) {
			return "uncompilable regexp (fail-closed); fix or use gitoid.PatternLower"
		}
		return ""
	}
	if !oidLengthQuant.MatchString(norm) {
		return ""
	}
	p40, p64 := oidProbe40(), oidProbe64()
	got40 := re.FindString(p40) == p40
	got64 := re.FindString(p64) == p64
	switch {
	case got40 && got64:
		return ""
	case got40 && !got64:
		return "pattern accepts 40-hex only; use gitoid.Valid (40|64)"
	case got64 && !got40:
		return "pattern accepts 64-hex only; use gitoid.Valid (40|64)"
	default:
		if found := re.FindString(p64); found != "" && found != p64 && re.FindString(p40) == p40 {
			return "pattern FindString(64hex)!=64hex; use gitoid.Valid (40|64)"
		}
		return ""
	}
}

func oidRegexShaped(s string) bool {
	return oidHexClass.MatchString(s) || oidLengthQuant.MatchString(normalizeBRE(s)) ||
		strings.ContainsAny(s, `^$`)
}

func normalizeBRE(s string) string {
	return breQuant.ReplaceAllString(s, `{$1}`)
}

func litIsGitoidWidth(lit *ast.BasicLit) bool {
	if lit == nil || lit.Kind != token.INT {
		return false
	}
	return lit.Value == strconv.Itoa(gitoid.LenSHA1) || lit.Value == strconv.Itoa(gitoid.LenSHA256)
}

// oidWidthCompareReason flags any relational compare against LenSHA1/LenSHA256.
func oidWidthCompareReason(bin *ast.BinaryExpr) string {
	switch bin.Op {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
	default:
		return ""
	}
	if litIsGitoidWidth(asBasicLit(bin.Y)) || litIsGitoidWidth(asBasicLit(bin.X)) {
		return "comparison against gitoid hash width; use gitoid.LenSHA1/LenSHA256 or gitoid.Valid"
	}
	return ""
}

func asBasicLit(e ast.Expr) *ast.BasicLit {
	lit, ok := e.(*ast.BasicLit)
	if !ok {
		return nil
	}
	return lit
}

// oidWidthConstReasons flags const/var int literals equal to a gitoid width —
// any name (plant: const n = 40).
func oidWidthConstReasons(spec *ast.ValueSpec) []oidHitReason {
	var out []oidHitReason
	for i, name := range spec.Names {
		if name == nil || i >= len(spec.Values) {
			continue
		}
		lit, ok := spec.Values[i].(*ast.BasicLit)
		if !ok || !litIsGitoidWidth(lit) {
			continue
		}
		out = append(out, oidHitReason{
			msg:    "const/var " + name.Name + " = " + lit.Value + " encodes gitoid hash width; use gitoid.LenSHA1/LenSHA256",
			needle: name.Name,
		})
	}
	return out
}

var schemaPatternLine = regexp.MustCompile(`"pattern"\s*:\s*"((?:\\.|[^"\\])*)"`)

func checkSchemaObjectIDPatterns(root string, idx *allowIndex) ([]string, error) {
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
		// Validate JSON, then scan per-line so duplicate pattern values keep
		// distinct line numbers (map last-wins would leave sibling waivers unused).
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		_ = doc
		for i, line := range strings.Split(string(data), "\n") {
			m := schemaPatternLine.FindStringSubmatch(line)
			if len(m) != 2 {
				continue
			}
			pat, err := strconv.Unquote(`"` + m[1] + `"`)
			if err != nil {
				pat = m[1]
			}
			lineNo := i + 1
			if oidPatternAcceptsBoth(pat) {
				continue
			}
			for _, cand := range oidPatternCandidates(pat) {
				reason := oidPatternBanReason(cand)
				if reason == "" {
					continue
				}
				if idx.allowed(rel, allowObjectID, lineNo, cand) || idx.allowed(rel, allowObjectID, lineNo, pat) {
					continue
				}
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, lineNo, reason))
			}
		}
	}
	return hits, nil
}

func checkTextObjectIDPatterns(root string, idx *allowIndex) ([]string, error) {
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
			if textSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		// Pattern-definition + selftest fixtures live under scripts/repoguard;
		// narrowing further is #427.
		if underDir(rel, "scripts/repoguard") {
			return nil
		}
		for _, skip := range textSkipDirList() {
			if underDir(rel, skip) || rel == skip {
				return nil
			}
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
		toks, tokErr := textTokensFailClosed(string(data), ext)
		if tokErr != nil {
			hits = append(hits, fmt.Sprintf("%s: tokenizer fail-closed: %v", rel, tokErr))
			return nil
		}
		for _, t := range toks {
			if idx.allowed(rel, allowObjectID, t.line, t.tok) || oidPatternAcceptsBoth(t.tok) {
				continue
			}
			// Bare 40-hex tokens in shell (plant); 64-hex content digests stay #427.
			if (ext == ".sh" || ext == ".bash") && isExactHexWidth(t.tok, gitoid.LenSHA1) {
				hits = append(hits, fmt.Sprintf("%s:%d: 40-hex token encodes gitoid width; use gitoid.PadSHA1 or LenSHA1",
					rel, t.line))
				continue
			}
			for _, cand := range oidPatternCandidates(t.tok) {
				reason := oidPatternBanReason(cand)
				if reason == "" {
					continue
				}
				if idx.allowed(rel, allowObjectID, t.line, cand) {
					continue
				}
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, t.line, reason))
			}
		}
		return nil
	})
	return hits, err
}

type textTok struct {
	line int
	tok  string
}

// textTokensFailClosed extracts quoted, awk-slash, =~, unquoted regex-looking
// shell args, and YAML `pattern:` / `run:` scalars. Unbalanced quotes/heredocs → error.
func textTokensFailClosed(src, ext string) ([]textTok, error) {
	if err := checkBalancedQuotesAndHeredocs(src, ext); err != nil {
		return nil, err
	}
	var out []textTok
	seen := map[string]bool{}
	add := func(line int, s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strconv.Itoa(line) + "\x00" + s
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, textTok{line: line, tok: s})
	}
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lineNo := i + 1
		for _, m := range regexp.MustCompile(
			`(?:r?'''([\s\S]*?)'''|r?"""([\s\S]*?)"""|r?'([^']*)'|r?"([^"]*)"|'([^']*)'|"([^"]*)"|`+
				"`"+`([^`+"`"+`]*)`+"`"+`)`,
		).FindAllStringSubmatch(line, -1) {
			for _, g := range m[1:] {
				if g != "" {
					add(lineNo, g)
				}
			}
		}
		for _, m := range regexp.MustCompile(`/(?:[^/\\]|\\.)+/`).FindAllString(line, -1) {
			if len(m) >= 2 {
				add(lineNo, m[1:len(m)-1])
			}
		}
		if idx := strings.Index(line, "=~"); idx >= 0 {
			rhs := strings.TrimSpace(line[idx+2:])
			rhs = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(rhs, "]]")), "]")
			if rhs != "" && !strings.HasPrefix(rhs, "'") && !strings.HasPrefix(rhs, "\"") {
				fields := strings.Fields(rhs)
				if len(fields) > 0 {
					add(lineNo, fields[0])
				}
			}
		}
		// Unquoted regex-looking shell args (plant: grep -E ^[0-9a-f]{40}$)
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.HasPrefix(f, "'") || strings.HasPrefix(f, "\"") || strings.HasPrefix(f, "`") {
				continue
			}
			// Strip trailing shell/awk punctuation glued to the token (e.g. /pat$/)).
			f = strings.TrimRight(f, ");,{")
			if f == "" {
				continue
			}
			if oidRegexShaped(f) || oidLengthQuant.MatchString(normalizeBRE(f)) {
				add(lineNo, f)
			}
		}
		// YAML scalars: pattern: ... or run: ...
		if m := regexp.MustCompile(`(?i)^\s*(?:-\s*)?(pattern|run)\s*:\s*(.+?)\s*$`).FindStringSubmatch(line); len(m) == 3 {
			val := strings.TrimSpace(m[2])
			val = strings.Trim(val, `"'`)
			add(lineNo, val)
		}
	}
	return out, nil
}

func checkBalancedQuotesAndHeredocs(src, ext string) error {
	// Whole-file scan: strip # comments outside strings; track triple quotes
	// across lines; treat mid-word apostrophes as text; ignore backticks
	// (markdown fences / shell cmdsubst — not OID quote delimiters).
	// Unterminated single/double/triple quote or heredoc → fail-closed.
	//
	// Escape rules differ by language:
	//   .py  — backslash escapes inside BOTH ' and " (and triples)
	//   .sh  — backslash escapes inside " only; ' is literal until '
	//          outside quotes, \X is an escaped literal (case patterns)
	//   .yml — treat like shell for ' / "; no heredocs
	py := ext == ".py"
	shellish := ext == ".sh" || ext == ".bash" || ext == ".yml" || ext == ".yaml"
	lines := strings.Split(src, "\n")
	var heredocCloser string
	var triple rune // 0 | '\'' | '"'
	var single, double bool
	for _, line := range lines {
		if heredocCloser != "" {
			if strings.TrimRight(line, "\r") == heredocCloser {
				heredocCloser = ""
			}
			continue
		}
		if shellish && !single && !double && triple == 0 {
			if m := regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`).FindStringSubmatch(line); len(m) == 2 {
				heredocCloser = m[1]
				continue
			}
		}
		runes := []rune(line)
		for j := 0; j < len(runes); j++ {
			r := runes[j]
			if triple != 0 {
				if py && r == '\\' {
					j++ // python triple: backslash escapes
					continue
				}
				if r == triple && j+2 < len(runes) && runes[j+1] == triple && runes[j+2] == triple {
					triple = 0
					j += 2
				}
				continue
			}
			if single {
				if py && r == '\\' {
					j++ // python: \' keeps the string open
					continue
				}
				if r == '\'' {
					single = false
				}
				continue
			}
			if double {
				if r == '\\' {
					j++
					continue
				}
				if r == '"' {
					double = false
				}
				continue
			}
			// Outside quotes: # starts a line comment (shell/yaml/python).
			if r == '#' {
				break
			}
			// Shell escape outside quotes: \' does not open a string
			// (case patterns like *[\'\"$`\\*]*). Python has no equivalent.
			if shellish && r == '\\' && j+1 < len(runes) {
				j++
				continue
			}
			// Python raw-string prefix r/R before quote.
			if py && (r == 'r' || r == 'R') && j+1 < len(runes) {
				nxt := runes[j+1]
				if nxt == '\'' || nxt == '"' {
					// Fall through to quote handling on next iter by
					// treating r"..." as starting at the quote — skip r.
					continue
				}
			}
			// Triple quotes open a spanning string.
			if (r == '\'' || r == '"') && j+2 < len(runes) && runes[j+1] == r && runes[j+2] == r {
				triple = r
				j += 2
				continue
			}
			switch r {
			case '\'':
				// Mid-word apostrophe: don't / Armature's / caller's
				if j > 0 && j+1 < len(runes) &&
					(unicode.IsLetter(runes[j-1]) || unicode.IsDigit(runes[j-1])) &&
					(unicode.IsLetter(runes[j+1]) || unicode.IsDigit(runes[j+1])) {
					continue
				}
				single = true
			case '"':
				double = true
			}
		}
	}
	if heredocCloser != "" {
		return fmt.Errorf("unterminated heredoc %s", heredocCloser)
	}
	if triple != 0 || single || double {
		return fmt.Errorf("unterminated quote")
	}
	return nil
}

func oidPatternBanned(s string) bool { return oidPatternBanReason(s) != "" }
