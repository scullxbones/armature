package scopematch

import (
	"strings"
	"testing"
	"time"
)

func mustOverlaps(t *testing.T, a, b string) bool {
	t.Helper()
	ok, err := Overlaps(a, b)
	if err != nil {
		t.Fatalf("Overlaps(%q, %q): %v", a, b, err)
	}
	return ok
}

func TestOverlaps_IgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if mustOverlaps(t, "docs/agents/quality-gates.md", "docs/use-cases.md") {
		t.Fatal("distinct files under an ancestor/descendant directory relationship must not overlap")
	}
	if mustOverlaps(t, "docs/use-cases.md", "docs/agents/quality-gates.md") {
		t.Fatal("overlap check must be symmetric")
	}
	if mustOverlaps(t, "internal/claim/a.go", "internal/claim/b.go") {
		t.Fatal("two distinct files in the same directory must not overlap merely by sharing that directory")
	}
	if mustOverlaps(t, "internal/claim/sub/*.go", "internal/claim/*.go") {
		t.Fatal("a single-segment glob must not overlap a deeper literal directory via ancestry alone")
	}
}

func TestOverlaps_StillMatchesGenuineOverlaps_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if !mustOverlaps(t, "README.md", "README.md") {
		t.Fatal("identical scope entries must overlap")
	}
	if !mustOverlaps(t, "internal/claim/*.go", "internal/claim/a.go") {
		t.Fatal("a glob must overlap a literal file it matches")
	}
	if !mustOverlaps(t, "internal/claim/a.go", "internal/claim/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
	if !mustOverlaps(t, "internal/claim/**", "internal/claim/sub/a.go") {
		t.Fatal("a doublestar glob spanning directories must overlap a nested file")
	}
	if !mustOverlaps(t, "docs/agents/", "docs/agents/quality-gates.md") {
		t.Fatal("a trailing-slash directory scope must overlap a file beneath it")
	}
}

func TestOverlaps_GlobVsGlobIntersection_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if !mustOverlaps(t, "src/auth/*.go", "src/auth/login.*") {
		t.Fatal("src/auth/*.go and src/auth/login.* both match src/auth/login.go and must overlap")
	}
	if !mustOverlaps(t, "src/auth/login.*", "src/auth/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
}

func TestOverlaps_GlobVsGlobNoIntersection_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if mustOverlaps(t, "src/auth/*.go", "src/billing/*.go") {
		t.Fatal("src/auth and src/billing are distinct literal directories; these globs cannot intersect")
	}
	if mustOverlaps(t, "src/billing/*.go", "src/auth/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
}

func TestOverlaps_WildcardDirectoryScopeIncludesDescendants(t *testing.T) {
	t.Parallel()
	if !mustOverlaps(t, "src/*/", "src/auth/login.go") {
		t.Fatal("src/*/ is a directory scope matching src/auth/, so it must overlap src/auth/login.go")
	}
	if !mustOverlaps(t, "src/auth/login.go", "src/*/") {
		t.Fatal("overlap check must be symmetric")
	}
	if !mustOverlaps(t, "src/*/", "src/billing/*.go") {
		t.Fatal("src/*/ covers every file under any src/<dir>/, including src/billing/*.go")
	}
	if mustOverlaps(t, "src/*/", "lib/foo.go") {
		t.Fatal("src/*/ must not overlap a path outside src/")
	}
}

func TestOverlaps_RepeatedDoublestarDoesNotHang(t *testing.T) {
	t.Parallel()
	const reps = 10
	a := strings.Repeat("**/a/", reps) + "x"
	b := strings.Repeat("**/a/", reps) + "y"
	type result struct {
		ab, ba bool
		err    error
	}
	done := make(chan result, 1)
	go func() {
		ab, err1 := Overlaps(a, b)
		ba, err2 := Overlaps(b, a)
		err := err1
		if err == nil {
			err = err2
		}
		done <- result{ab, ba, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Overlaps on repeated **/a: %v", got.err)
		}
		if got.ab {
			t.Fatal("repeated **/a ending in distinct literals must not intersect")
		}
		if got.ba {
			t.Fatal("overlap check must be symmetric")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Overlaps hung on repeated **/a patterns; intersection must memoize suffix-index pairs")
	}
}

func TestGlobPatternsMayIntersect_RootScope(t *testing.T) {
	t.Parallel()
	if !globPatternsMayIntersect(".", "src/foo.go") {
		t.Fatal("repo-root scope must intersect any path")
	}
	if !globPatternsMayIntersect("src/foo.go", ".") {
		t.Fatal("intersection with repo-root must be symmetric")
	}
}

func TestMemoizedSuffixIntersection_DoublestarBacktracking(t *testing.T) {
	t.Parallel()

	if !memoizedSuffixIntersection([]string{"**", "x"}, []string{"a", "b", "x"}) {
		t.Fatal("** in a should backtrack to consume [a b] and match trailing x")
	}
	if memoizedSuffixIntersection([]string{"**", "x"}, []string{"a", "b", "y"}) {
		t.Fatal("** in a should exhaust backtracking and report no match when trailing segment never matches")
	}
	if !memoizedSuffixIntersection([]string{"**"}, []string{"anything", "at", "all"}) {
		t.Fatal("a lone ** segment must match any remaining segments, including none")
	}
	if !memoizedSuffixIntersection([]string{"**"}, nil) {
		t.Fatal("a lone ** segment must match zero remaining segments")
	}

	if !memoizedSuffixIntersection([]string{"a", "b", "x"}, []string{"**", "x"}) {
		t.Fatal("** in b should backtrack to consume [a b] and match trailing x")
	}
	if memoizedSuffixIntersection([]string{"a", "b", "y"}, []string{"**", "x"}) {
		t.Fatal("** in b should exhaust backtracking and report no match when trailing segment never matches")
	}
	if !memoizedSuffixIntersection([]string{"anything", "at", "all"}, []string{"**"}) {
		t.Fatal("a lone ** segment in b must match any remaining segments, including none")
	}
	if !memoizedSuffixIntersection(nil, []string{"**"}) {
		t.Fatal("a lone ** segment in b must match zero remaining segments")
	}

	if memoizedSuffixIntersection([]string{"a", "b"}, []string{"a"}) {
		t.Fatal("differing segment counts without ** must not match")
	}
}

func TestConservativelyCompatibleSegments_AllBranches(t *testing.T) {
	t.Parallel()

	if !conservativelyCompatibleSegments("x.go", "x.go") {
		t.Fatal("identical literal segments must be compatible")
	}
	if conservativelyCompatibleSegments("a.go", "b.go") {
		t.Fatal("distinct literal segments must not be compatible")
	}
	if !conservativelyCompatibleSegments("*.go", "a.go") {
		t.Fatal("wildcard segment matching a literal segment must be compatible")
	}
	if conservativelyCompatibleSegments("*.go", "a.py") {
		t.Fatal("wildcard segment not matching a literal segment must not be compatible")
	}
	if !conservativelyCompatibleSegments("a.go", "*.go") {
		t.Fatal("literal segment matched by a wildcard segment (args reversed) must be compatible")
	}
	if conservativelyCompatibleSegments("a.py", "*.go") {
		t.Fatal("literal segment not matched by a wildcard segment (args reversed) must not be compatible")
	}
	if !conservativelyCompatibleSegments("*.go", "login.*") {
		t.Fatal("two wildcard segments must be conservatively treated as compatible")
	}
}

func TestAllows_RootScopeMatchesAnyPath(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"."}, "internal/foo.go") {
		t.Fatal("expected root scope '.' to match any path")
	}
}

func TestAllows_ExactPathMatch(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"internal/foo.go"}, "internal/foo.go") {
		t.Fatal("expected exact path match")
	}
	if Allows([]string{"internal/foo.go"}, "internal/bar.go") {
		t.Fatal("expected no match for different path")
	}
}

func TestAllows_WildcardDirectoryScopeIncludesDescendants(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"src/*/"}, "src/auth/login.go") {
		t.Fatal("expected src/*/ directory scope to cover descendants of a matched directory")
	}
	if Allows([]string{"src/*/"}, "lib/foo.go") {
		t.Fatal("expected src/*/ not to cover a path outside src/")
	}
}

func TestAllows_TrailingSlashDirectoryScope(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"internal/"}, "internal/foo.go") {
		t.Fatal("expected trailing-slash directory scope to cover file directly inside it")
	}
	if !Allows([]string{"internal/"}, "internal/sub/foo.go") {
		t.Fatal("expected trailing-slash directory scope to cover nested files")
	}
	if Allows([]string{"internal/"}, "other/foo.go") {
		t.Fatal("expected trailing-slash directory scope to not cover unrelated path")
	}
	if Allows([]string{"internal"}, "internal2/foo.go") {
		t.Fatal("expected exact directory entry without trailing slash to not match a differently-named sibling")
	}
}

func TestAllows_DotSlashPrefixNormalized(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"./internal/**"}, "internal/foo.go") {
		t.Fatal("expected './internal/**' scope entry to match 'internal/foo.go' after normalization")
	}
}

func TestAllows_DoublestarMatchesNestedPaths(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"internal/**"}, "internal/foo/bar.go") {
		t.Fatal("expected 'internal/**' to match nested path")
	}
	if !Allows([]string{"internal/**/api.go"}, "internal/foo/bar/api.go") {
		t.Fatal("expected mid-pattern '**' to span multiple segments")
	}
	if Allows([]string{"internal/**/api.go"}, "internal/foo/bar/other.go") {
		t.Fatal("expected mid-pattern '**' pattern to still require the literal suffix segment to match")
	}
}

func TestAllows_SingleSegmentGlob(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"internal/*.go"}, "internal/foo.go") {
		t.Fatal("expected single-segment glob to match file directly inside dir")
	}
	if Allows([]string{"internal/*.go"}, "internal/sub/foo.go") {
		t.Fatal("expected single-segment glob to not match nested file")
	}
}

func TestAllows_NoScopeEntriesMatches(t *testing.T) {
	t.Parallel()
	if Allows(nil, "internal/foo.go") {
		t.Fatal("expected empty scope to match nothing")
	}
}

func TestCleanRepoPath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		".":              ".",
		"./internal/foo": "internal/foo",
		"internal/foo":   "internal/foo",
		"/internal/foo":  "internal/foo",
		"internal//foo":  "internal/foo",
		"internal/./foo": "internal/foo",
	}
	for input, want := range cases {
		if got := CleanRepoPath(input); got != want {
			t.Fatalf("CleanRepoPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAllows_StripsNewFileAnnotation(t *testing.T) {
	t.Parallel()
	if !Allows([]string{"internal/foo.go (new)"}, "internal/foo.go") {
		t.Fatal("expected '(new)' annotation on scope entry to be stripped before matching")
	}
}

func TestCleanScope_StripsNewFileAnnotation(t *testing.T) {
	t.Parallel()
	cleaned, isDir := CleanScope("internal/foo.go (new)")
	if isDir {
		t.Fatal("expected annotated non-directory scope entry to not be a directory scope")
	}
	if cleaned != "internal/foo.go" {
		t.Fatalf("expected cleaned scope 'internal/foo.go', got %q", cleaned)
	}
}

func TestCleanScope_PreservesFilenameWithLiteralParens(t *testing.T) {
	t.Parallel()
	cleaned, _ := CleanScope("internal/foo/bar(baz).go")
	if cleaned != "internal/foo/bar(baz).go" {
		t.Fatalf("expected literal parens in filename to be preserved, got %q", cleaned)
	}
}

func TestOverlaps_MalformedPatternReturnsError_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	ok, err := Overlaps("foo[", "foo.go")
	if err == nil {
		t.Fatal("expected error for malformed scope pattern")
	}
	if ok {
		t.Fatal("malformed pattern must not report overlap")
	}
	if !strings.Contains(err.Error(), "foo[") {
		t.Fatalf("error must name the pattern, got %q", err)
	}

	ok, err = Overlaps("foo.go", "foo[")
	if err == nil {
		t.Fatal("expected error when the other side is malformed")
	}
	if ok {
		t.Fatal("malformed pattern must not report overlap")
	}
}

func TestOverlaps_ValidatesBothPatternsBeforeMatch_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	cases := []struct{ a, b string }{
		{"*", "["},
		{"[", "*"},
	}
	for _, tc := range cases {
		ok, err := Overlaps(tc.a, tc.b)
		if err == nil {
			t.Fatalf("Overlaps(%q, %q): expected error, got match=%v", tc.a, tc.b, ok)
		}
		if ok {
			t.Fatalf("Overlaps(%q, %q): malformed pattern must not report overlap", tc.a, tc.b)
		}
	}
}

func TestOverlaps_NoMatchIsNotError_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	ok, err := Overlaps("src/a.go", "src/b.go")
	if err != nil {
		t.Fatalf("valid non-overlapping patterns must not error: %v", err)
	}
	if ok {
		t.Fatal("distinct files must not overlap")
	}
}

func TestCleanScope_DetectsTrailingSlash(t *testing.T) {
	t.Parallel()
	cleaned, isDir := CleanScope("internal/")
	if !isDir {
		t.Fatal("expected trailing-slash scope to be detected as a directory scope")
	}
	if cleaned != "internal" {
		t.Fatalf("expected cleaned scope 'internal', got %q", cleaned)
	}

	cleaned, isDir = CleanScope("internal/foo.go")
	if isDir {
		t.Fatal("expected non-trailing-slash scope to not be a directory scope")
	}
	if cleaned != "internal/foo.go" {
		t.Fatalf("expected cleaned scope 'internal/foo.go', got %q", cleaned)
	}
}
