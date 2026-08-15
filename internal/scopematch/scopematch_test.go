package scopematch

import (
	"strings"
	"testing"
	"time"
)

// TestOverlaps_IgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T7 verifies the
// canonical implementation, added by LNGHZN-S10-T7 as the single source both
// internal/claim and internal/validate delegate to, does not fall back to
// "shares a containing/ancestor directory": two distinct files under the
// same or a nested directory must not be reported as overlapping.
func TestOverlaps_IgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if Overlaps("docs/agents/quality-gates.md", "docs/use-cases.md") {
		t.Fatal("distinct files under an ancestor/descendant directory relationship must not overlap")
	}
	if Overlaps("docs/use-cases.md", "docs/agents/quality-gates.md") {
		t.Fatal("overlap check must be symmetric")
	}
	if Overlaps("internal/claim/a.go", "internal/claim/b.go") {
		t.Fatal("two distinct files in the same directory must not overlap merely by sharing that directory")
	}
	if Overlaps("internal/claim/sub/*.go", "internal/claim/*.go") {
		t.Fatal("a single-segment glob must not overlap a deeper literal directory via ancestry alone")
	}
}

// TestOverlaps_StillMatchesGenuineOverlaps_REQ_LNGHZN_S10_T7 verifies that
// removing directory-ancestry matching did not weaken genuine overlap
// detection: identical paths, a glob matching a literal file, "**" spanning
// directories, and a trailing-slash directory scope must all still overlap.
func TestOverlaps_StillMatchesGenuineOverlaps_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if !Overlaps("README.md", "README.md") {
		t.Fatal("identical scope entries must overlap")
	}
	if !Overlaps("internal/claim/*.go", "internal/claim/a.go") {
		t.Fatal("a glob must overlap a literal file it matches")
	}
	if !Overlaps("internal/claim/a.go", "internal/claim/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
	if !Overlaps("internal/claim/**", "internal/claim/sub/a.go") {
		t.Fatal("a doublestar glob spanning directories must overlap a nested file")
	}
	if !Overlaps("docs/agents/", "docs/agents/quality-gates.md") {
		t.Fatal("a trailing-slash directory scope must overlap a file beneath it")
	}
}

// TestOverlaps_GlobVsGlobIntersection_REQ_LNGHZN_S10_T7 verifies that
// glob-vs-glob overlap is detected via pattern intersection, not just
// glob-vs-literal containment: "src/auth/*.go" and "src/auth/login.*" both
// match "src/auth/login.go" even though neither pattern matches the other's
// literal string. Found by automated review on PR #102 as a correctness gap
// in the T6 implementation — under-blocking here would let two claims with a
// genuine glob-vs-glob conflict proceed concurrently.
func TestOverlaps_GlobVsGlobIntersection_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if !Overlaps("src/auth/*.go", "src/auth/login.*") {
		t.Fatal("src/auth/*.go and src/auth/login.* both match src/auth/login.go and must overlap")
	}
	if !Overlaps("src/auth/login.*", "src/auth/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
}

// TestOverlaps_GlobVsGlobNoIntersection_REQ_LNGHZN_S10_T7 verifies the
// glob-vs-glob over-approximation stays bounded: two globs whose literal
// directory segments differ cannot possibly match a common path and must
// still report no overlap, or "any two globs overlap" would recreate the
// warning wall this whole line of work removed.
func TestOverlaps_GlobVsGlobNoIntersection_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()
	if Overlaps("src/auth/*.go", "src/billing/*.go") {
		t.Fatal("src/auth and src/billing are distinct literal directories; these globs cannot intersect")
	}
	if Overlaps("src/billing/*.go", "src/auth/*.go") {
		t.Fatal("overlap check must be symmetric")
	}
}

// TestOverlaps_WildcardDirectoryScopeIncludesDescendants verifies that a
// trailing-slash directory scope that also contains a wildcard (e.g.
// "src/*/") still covers descendants of any matched directory. CleanScope
// reports isDir, but throwing that flag away makes the segment matcher
// require equal length, and Allows' descendant-prefix check is a literal
// string prefix against "src/*/" — both miss src/auth/login.go.
func TestOverlaps_WildcardDirectoryScopeIncludesDescendants(t *testing.T) {
	t.Parallel()
	if !Overlaps("src/*/", "src/auth/login.go") {
		t.Fatal("src/*/ is a directory scope matching src/auth/, so it must overlap src/auth/login.go")
	}
	if !Overlaps("src/auth/login.go", "src/*/") {
		t.Fatal("overlap check must be symmetric")
	}
	if !Overlaps("src/*/", "src/billing/*.go") {
		t.Fatal("src/*/ covers every file under any src/<dir>/, including src/billing/*.go")
	}
	if Overlaps("src/*/", "lib/foo.go") {
		t.Fatal("src/*/ must not overlap a path outside src/")
	}
}

// TestOverlaps_RepeatedDoublestarDoesNotHang verifies that a pair of
// non-intersecting patterns with many "**" segments finishes in bounded
// time. Naive suffix-slice recursion revisits the same (i, j) states
// combinatorially; ten **/a repetitions already require hundreds of
// millions of calls and can hang arm claim / arm validate.
func TestOverlaps_RepeatedDoublestarDoesNotHang(t *testing.T) {
	t.Parallel()
	const reps = 10
	a := strings.Repeat("**/a/", reps) + "x"
	b := strings.Repeat("**/a/", reps) + "y"
	type result struct{ ab, ba bool }
	done := make(chan result, 1)
	go func() {
		done <- result{Overlaps(a, b), Overlaps(b, a)}
	}()
	select {
	case got := <-done:
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

// TestMatchPatternSegments_DoublestarBacktracking exercises the "**"
// backtracking loops in matchPatternSegments directly (white-box, same
// package) for both the a[0]=="**" and b[0]=="**" branches, including cases
// where the loop finds a match partway through and cases where it exhausts
// without finding one.
func TestGlobPatternsMayIntersect_RootScope(t *testing.T) {
	t.Parallel()
	if !globPatternsMayIntersect(".", "src/foo.go") {
		t.Fatal("repo-root scope must intersect any path")
	}
	if !globPatternsMayIntersect("src/foo.go", ".") {
		t.Fatal("intersection with repo-root must be symmetric")
	}
}

func TestMatchPatternSegments_DoublestarBacktracking(t *testing.T) {
	t.Parallel()

	// a[0] == "**", trailing pattern matches after consuming some segments.
	if !matchPatternSegments([]string{"**", "x"}, []string{"a", "b", "x"}) {
		t.Fatal("** in a should backtrack to consume [a b] and match trailing x")
	}
	// a[0] == "**", no valid backtrack position matches.
	if matchPatternSegments([]string{"**", "x"}, []string{"a", "b", "y"}) {
		t.Fatal("** in a should exhaust backtracking and report no match when trailing segment never matches")
	}
	// a == ["**"] alone (len(a)==1 short-circuit).
	if !matchPatternSegments([]string{"**"}, []string{"anything", "at", "all"}) {
		t.Fatal("a lone ** segment must match any remaining segments, including none")
	}
	if !matchPatternSegments([]string{"**"}, nil) {
		t.Fatal("a lone ** segment must match zero remaining segments")
	}

	// b[0] == "**", symmetric to the above.
	if !matchPatternSegments([]string{"a", "b", "x"}, []string{"**", "x"}) {
		t.Fatal("** in b should backtrack to consume [a b] and match trailing x")
	}
	if matchPatternSegments([]string{"a", "b", "y"}, []string{"**", "x"}) {
		t.Fatal("** in b should exhaust backtracking and report no match when trailing segment never matches")
	}
	if !matchPatternSegments([]string{"anything", "at", "all"}, []string{"**"}) {
		t.Fatal("a lone ** segment in b must match any remaining segments, including none")
	}
	if !matchPatternSegments(nil, []string{"**"}) {
		t.Fatal("a lone ** segment in b must match zero remaining segments")
	}

	// Neither side has "**": differing lengths never match.
	if matchPatternSegments([]string{"a", "b"}, []string{"a"}) {
		t.Fatal("differing segment counts without ** must not match")
	}
}

// TestSegmentsCompatible_AllBranches exercises every branch of
// segmentsCompatible directly: identical segments, literal-vs-literal
// mismatch, wildcard-vs-literal in both directions (match and mismatch), and
// wildcard-vs-wildcard (always conservatively compatible).
func TestSegmentsCompatible_AllBranches(t *testing.T) {
	t.Parallel()

	if !segmentsCompatible("x.go", "x.go") {
		t.Fatal("identical literal segments must be compatible")
	}
	if segmentsCompatible("a.go", "b.go") {
		t.Fatal("distinct literal segments must not be compatible")
	}
	if !segmentsCompatible("*.go", "a.go") {
		t.Fatal("wildcard segment matching a literal segment must be compatible")
	}
	if segmentsCompatible("*.go", "a.py") {
		t.Fatal("wildcard segment not matching a literal segment must not be compatible")
	}
	if !segmentsCompatible("a.go", "*.go") {
		t.Fatal("literal segment matched by a wildcard segment (args reversed) must be compatible")
	}
	if segmentsCompatible("a.py", "*.go") {
		t.Fatal("literal segment not matched by a wildcard segment (args reversed) must not be compatible")
	}
	if !segmentsCompatible("*.go", "login.*") {
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
	// Directory scope without trailing slash must not match by prefix
	// alone (e.g. "internal" should not match "internal2/foo.go").
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
