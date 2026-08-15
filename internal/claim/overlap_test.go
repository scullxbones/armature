package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/stretchr/testify/assert"
)

// TestScopesOverlap_ExcludesAncestorDescendantPairs_REQ_TOPTIER_S17_T1 verifies that
// ScopesOverlapEx excludes ancestor/descendant issue pairs from conflict comparison.
// A parent story's scope is by design the union of its children's scopes,
// so comparing a child against its parent should never be a conflict.
func TestScopesOverlap_ExcludesAncestorDescendantPairs_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

	// Build a simple hierarchy:
	// story-01 (scope: ["src/**"])
	//   └─ task-01 (scope: ["src/auth/**"])
	nodes := map[string]*dag.Node{
		"story-01": {
			ID:        "story-01",
			Title:     "Parent Story",
			Type:      "story",
			Parent:    "",
			Children:  []string{"task-01"},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
		"task-01": {
			ID:        "task-01",
			Title:     "Child Task",
			Type:      "task",
			Parent:    "story-01",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
	}
	graph := dag.FromIndex(nodes)

	// Even though the scopes overlap (task-01's scope is subset of story-01's),
	// ScopesOverlapEx should return false because they are parent and child
	scopeParent := []string{"src/**"}
	scopeChild := []string{"src/auth/**"}

	// Child claiming against parent should not report overlap
	result := ScopesOverlapEx(scopeChild, scopeParent, graph, "task-01", "story-01")
	assert.False(t, result, "child task should not conflict with parent story despite scope overlap")

	// Parent claiming against child should not report overlap either
	result = ScopesOverlapEx(scopeParent, scopeChild, graph, "story-01", "task-01")
	assert.False(t, result, "parent story should not conflict with child task despite scope overlap")

	// Non-ancestor/descendant pairs with overlapping scopes should still report overlap
	sibling := &dag.Node{
		ID:        "task-02",
		Title:     "Sibling Task",
		Type:      "task",
		Parent:    "story-01",
		Children:  []string{},
		BlockedBy: []string{},
		Blocks:    []string{},
	}
	nodes["task-02"] = sibling
	graph = dag.FromIndex(nodes)

	result = ScopesOverlapEx(scopeChild, scopeChild, graph, "task-01", "task-02")
	assert.True(t, result, "sibling tasks with same scope should conflict")
}

// TestScopesOverlap_StillDetectsNonAncestorOverlaps_REQ_TOPTIER_S17_T1 verifies that
// non-ancestor/descendant pairs with overlapping scopes are still detected.
func TestScopesOverlap_StillDetectsNonAncestorOverlaps_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

	// Build a graph with unrelated tasks
	nodes := map[string]*dag.Node{
		"task-a": {
			ID:        "task-a",
			Title:     "Task A",
			Type:      "task",
			Parent:    "",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
		"task-b": {
			ID:        "task-b",
			Title:     "Task B",
			Type:      "task",
			Parent:    "",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
	}
	graph := dag.FromIndex(nodes)

	// Tasks with overlapping scopes should still conflict
	scopeA := []string{"src/auth/**"}
	scopeB := []string{"src/auth/login.go"}

	result := ScopesOverlapEx(scopeA, scopeB, graph, "task-a", "task-b")
	assert.True(t, result, "unrelated tasks with overlapping scopes should conflict")
}

func TestGlobOverlaps_RespectsPathSegmentBoundaries_PR79(t *testing.T) {
	t.Parallel()
	// internal/claimx has internal/claim as a *string* prefix but is not nested
	// under it as a path segment. A naive hasPrefix(dirA, dirB) check falsely
	// treats these as overlapping. Regression coverage for the bug found in
	// fable's holistic review of PR #79.
	assert.False(t, globOverlaps("internal/claimx/foo.go", "internal/claim/*.go"),
		"internal/claimx and internal/claim share a string prefix but are sibling directories, not nested — must not overlap")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claimx/foo.go"),
		"overlap check must be symmetric")

	// NOTE(LNGHZN-S10-T6): these two cases previously asserted `true` on the
	// strength of the now-removed containing/ancestor-directory fallback
	// (dirA == dirB, or one a path-segment prefix of the other). Per
	// LNGHZN-S10-T6, overlap is now decided by exact path or glob match
	// only, so two glob patterns or two literal files that merely share a
	// directory no longer overlap unless one pattern actually matches the
	// other (e.g. a "**" or trailing-slash directory scope).
	assert.False(t, globOverlaps("internal/claim/sub/*.go", "internal/claim/*.go"),
		"single-segment glob 'internal/claim/*.go' does not match the deeper literal directory 'sub/' — no longer treated as overlapping via directory ancestry")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claim/sub/*.go"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/a.go", "internal/claim/b.go"),
		"two distinct literal files that merely share a containing directory must not overlap")
}

// TestGlobOverlapsIgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T6 verifies that
// globOverlaps no longer reports overlap for two distinct files that merely
// share a containing or ancestor directory. This is the regression coverage
// for the bug reported in
// docs/dogfood/findings/raw/2026-08-14T2352Z-5207ee28-tooling-scope-overlap-matches-on-directory-not-file.md,
// where docs/agents/quality-gates.md (dir "docs/agents") and
// docs/use-cases.md (dir "docs") were falsely reported as overlapping
// because "docs/agents" has "docs/" as a string prefix.
func TestGlobOverlapsIgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("docs/agents/quality-gates.md", "docs/use-cases.md"),
		"distinct files under an ancestor/descendant directory relationship must not overlap")
	assert.False(t, globOverlaps("docs/use-cases.md", "docs/agents/quality-gates.md"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/overlap.go", "internal/claim/overlap_test.go"),
		"two distinct files in the same directory must not overlap merely by sharing that directory")
	assert.False(t, globOverlaps("internal/claim/overlap_test.go", "internal/claim/overlap.go"),
		"overlap check must be symmetric")
}

// TestOverlapDetectsGlobToGlobIntersection_REQ_LNGHZN_S10_T6 verifies that
// two glob patterns in the same directory that could both match a common
// filename are reported as overlapping, even though neither pattern
// literally matches the other (filepath.Match(a, b) and
// filepath.Match(b, a) are both false, and neither pattern's file, treated
// literally, satisfies the other via scopematch.Allows). This is the
// dangerous under-blocking direction the directory-fallback removal
// exposed: "src/auth/*.go" and "src/auth/login.*" both match
// "src/auth/login.go", so two workers claiming these scopes concurrently
// could both write that file.
func TestOverlapDetectsGlobToGlobIntersection_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/auth/*.go", "src/auth/login.*"),
		"both patterns can match src/auth/login.go and must be reported as overlapping")
	assert.True(t, globOverlaps("src/auth/login.*", "src/auth/*.go"),
		"overlap check must be symmetric")
}

// TestOverlapStripsNewFileAnnotation_REQ_LNGHZN_S10_T6 verifies that a
// worker-declared " (new)" annotation does not hide a real overlap:
// "src/foo.go (new)" and "src/*.go" both cover src/foo.go.
func TestOverlapStripsNewFileAnnotation_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/foo.go (new)", "src/*.go"),
		"annotated new-file scope must overlap the glob that covers that file")
	assert.True(t, globOverlaps("src/*.go", "src/foo.go (new)"),
		"overlap check must be symmetric")
	assert.False(t, globOverlaps("src/foo.go (new)", "src/bar.go"),
		"annotation stripping must not invent overlap between distinct files")
}

// TestOverlapIntersectsDoublestarAcrossDirectories_REQ_LNGHZN_S10_T6 verifies
// that glob-to-glob intersection walks the full path, not just equal
// directory prefixes. src/**/foo.go and src/auth/*.go both include
// src/auth/foo.go, even though their literal directory strings differ.
func TestOverlapIntersectsDoublestarAcrossDirectories_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/**/foo.go", "src/auth/*.go"),
		"src/**/foo.go and src/auth/*.go both match src/auth/foo.go")
	assert.True(t, globOverlaps("src/auth/*.go", "src/**/foo.go"),
		"overlap check must be symmetric")
	assert.False(t, globOverlaps("src/**/foo.go", "src/auth/*.txt"),
		"foo.go cannot intersect a *.txt glob in the same directory")
}

// TestOverlapIntersectsCharacterClass_REQ_LNGHZN_S10_T6 verifies that
// filepath.Match character classes participate in glob intersection:
// src/file[ab].go and src/filea.* both match src/filea.go.
func TestOverlapIntersectsCharacterClass_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/file[ab].go", "src/filea.*"),
		"file[ab].go and filea.* both match src/filea.go")
	assert.True(t, globOverlaps("src/filea.*", "src/file[ab].go"),
		"overlap check must be symmetric")
}

// TestGlobPatternsIntersect_REQ_LNGHZN_S10_T6 directly exercises
// globPatternsIntersect's branches: "*" wildcards on either side, "?"
// single-character wildcards, literal equality, literal mismatch, and the
// case where one pattern is a plain literal fully consumed while the other
// still has remaining non-"*" characters (which must not intersect).
func TestGlobPatternsIntersect_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical literals", "login.go", "login.go", true},
		{"different literals, same length", "login.go", "logout.go", false},
		{"leading star vs literal suffix", "*.go", "login.go", true},
		{"trailing star vs literal prefix", "login.*", "login.go", true},
		{"star vs star, different fixed suffixes cannot intersect", "*.go", "*.txt", false},
		{"star vs star, same fixed suffix intersects", "*.go", "a*.go", true},
		{"question mark matches any single char", "login?go", "login.go", true},
		{"question mark on both sides", "l?gin.go", "log?n.go", true},
		{"literal fully consumed but other side has trailing literal", "login", "login.go", false},
		{"disjoint literal suffixes with stars", "*.go", "*.txt.go", true},
		{"no possible common length", "ab", "abc", false},
		{"character class vs matching literal", "file[ab].go", "filea.go", true},
		{"character class vs star suffix", "file[ab].go", "filea.*", true},
		{"unclosed class is a literal bracket", "file[ab.go", "file[ab.go", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, globPatternsIntersect(c.a, c.b), "globPatternsIntersect(%q, %q)", c.a, c.b)
			assert.Equal(t, c.want, globPatternsIntersect(c.b, c.a), "globPatternsIntersect(%q, %q) (symmetric)", c.b, c.a)
		})
	}
}

// TestOverlapGlobToGlobIntersectionBounded_REQ_LNGHZN_S10_T6 verifies the
// glob-to-glob over-approximation is bounded by path-segment intersection,
// not "any two globs overlap": two globs that cannot match a common path
// (different literal directories, no "**") are still reported as
// non-overlapping.
func TestOverlapGlobToGlobIntersectionBounded_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("src/auth/*.go", "src/billing/*.go"),
		"different directories can never share a matched file, regardless of filename pattern")
	assert.False(t, globOverlaps("src/billing/*.go", "src/auth/*.go"),
		"overlap check must be symmetric")
}

// TestOverlapGlobToGlobIntersectionRegression_REQ_LNGHZN_S10_T6 re-confirms,
// in both directions, that the glob-to-glob intersection fallback did not
// resurrect the removed ancestor/containing-directory fallback: entries
// whose directory portions differ still never overlap merely by proximity.
func TestOverlapGlobToGlobIntersectionRegression_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("docs/agents/quality-gates.md", "docs/use-cases.md"))
	assert.False(t, globOverlaps("docs/use-cases.md", "docs/agents/quality-gates.md"))

	assert.False(t, globOverlaps("internal/claim/overlap.go", "internal/claim/overlap_test.go"))
	assert.False(t, globOverlaps("internal/claim/overlap_test.go", "internal/claim/overlap.go"))
}

// TestGlobOverlapsStillMatchesIdenticalAndGlobScopes_REQ_LNGHZN_S10_T6 verifies
// that removing the directory fallback did not break genuine overlap
// detection: identical scope entries still overlap, and an explicit
// directory glob like "docs/agents/**" still reports overlap against a file
// beneath it.
func TestGlobOverlapsStillMatchesIdenticalAndGlobScopes_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("README.md", "README.md"),
		"identical scope entries must still overlap")

	assert.True(t, globOverlaps("docs/agents/**", "docs/agents/quality-gates.md"),
		"an explicit directory glob must still overlap a file beneath it")
	assert.True(t, globOverlaps("docs/agents/quality-gates.md", "docs/agents/**"),
		"overlap check must be symmetric")
}

// globOverlapParityCases mirrors the identically-named table in
// internal/validate/validate_test.go. The two globOverlaps implementations
// are intentionally duplicated (validate cannot import claim per the
// validate-boundary depguard rule in .golangci.yml) and must be kept
// behaviorally identical — if you change this package's matching semantics,
// update this table AND the matching table in
// internal/validate/validate_test.go so the parity test there catches drift.
var globOverlapParityCases = []struct {
	name string
	a, b string
	want bool
}{
	{"exact match", "internal/claim/a.go", "internal/claim/a.go", true},
	{"glob vs literal in dir", "internal/claim/*.go", "internal/claim/a.go", true},
	{"sibling dir string-prefix, no overlap", "internal/claimx/foo.go", "internal/claim/*.go", false},
	// LNGHZN-S10-T6: previously "true" under the now-removed
	// containing/ancestor-directory fallback. "internal/claim/*.go" is a
	// single-segment glob and does not match the deeper literal path
	// "internal/claim/sub/*.go", so these no longer overlap.
	{"no longer overlaps via directory nesting alone", "internal/claim/sub/*.go", "internal/claim/*.go", false},
	{"unrelated dirs", "internal/claim/a.go", "internal/validate/a.go", false},
	{"root-level files, no dir", "a.go", "b.go", false},
	{"explicit doublestar directory glob still overlaps nested file", "internal/claim/**", "internal/claim/sub/a.go", true},
}

func TestGlobOverlaps_ParityWithValidatePackage_PR79(t *testing.T) {
	t.Parallel()
	for _, c := range globOverlapParityCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, globOverlaps(c.a, c.b), "globOverlaps(%q, %q)", c.a, c.b)
			assert.Equal(t, c.want, globOverlaps(c.b, c.a), "globOverlaps(%q, %q) (symmetric)", c.b, c.a)
		})
	}
}

// TestIsWithinScope_FilesWithinDeclaredScope_REQ_LNGHZN_S4_T1 verifies that IsWithinScope
// correctly identifies files that are within the declared scope globs.
func TestIsWithinScope_FilesWithinDeclaredScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		files    []string
		scope    []string
		wantIsIn bool
		wantFile string // first file that's out of scope (empty if all in)
	}{
		{
			name:     "all files in single glob pattern",
			files:    []string{"internal/claim/overlap.go", "internal/claim/overlap_test.go"},
			scope:    []string{"internal/claim/**"},
			wantIsIn: true,
		},
		{
			name:     "all files in multiple glob patterns",
			files:    []string{"internal/claim/overlap.go", "cmd/armature/main.go"},
			scope:    []string{"internal/claim/**", "cmd/armature/**"},
			wantIsIn: true,
		},
		{
			name:     "single file in single exact pattern",
			files:    []string{"internal/claim/overlap.go"},
			scope:    []string{"internal/claim/overlap.go"},
			wantIsIn: true,
		},
		{
			name:     "file outside scope",
			files:    []string{"internal/validate/validate.go"},
			scope:    []string{"internal/claim/**"},
			wantIsIn: false,
			wantFile: "internal/validate/validate.go",
		},
		{
			name:     "mixed files with one outside scope",
			files:    []string{"internal/claim/overlap.go", "internal/validate/validate.go"},
			scope:    []string{"internal/claim/**"},
			wantIsIn: false,
			wantFile: "internal/validate/validate.go",
		},
		{
			name:     "empty files list within any scope",
			files:    []string{},
			scope:    []string{"internal/claim/**"},
			wantIsIn: true,
		},
		{
			name:     "directory glob patterns",
			files:    []string{"internal/claim/a.go", "internal/claim/sub/b.go"},
			scope:    []string{"internal/claim/**"},
			wantIsIn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			isIn, outFile := IsWithinScope(tt.files, tt.scope)
			assert.Equal(t, tt.wantIsIn, isIn, "IsWithinScope(%v, %v) isIn", tt.files, tt.scope)
			if !tt.wantIsIn && tt.wantFile != "" {
				assert.Equal(t, tt.wantFile, outFile, "IsWithinScope(%v, %v) out of scope file", tt.files, tt.scope)
			}
		})
	}
}

// TestIsWithinScope_CaseSensitivity_REQ_LNGHZN_S4_T1 verifies that IsWithinScope
// respects case-sensitive matching (like glob.Match on Unix).
func TestIsWithinScope_CaseSensitivity_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	// Case-sensitive matching: "internal/Claim" should not match "internal/claim/**"
	isIn, _ := IsWithinScope([]string{"internal/Claim/overlap.go"}, []string{"internal/claim/**"})
	assert.False(t, isIn, "case mismatch should not match on Unix")
}

// TestIsWithinScope_StripsNewFileAnnotation_REQ_LNGHZN_S4_T2 verifies that a
// scope entry carrying the " (new)" annotation workers commonly append when
// declaring a file that doesn't exist yet (e.g. "internal/foo/bar.go (new)")
// still matches the real file path once it's created.
func TestIsWithinScope_StripsNewFileAnnotation_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/deliverygate/gate.go", "internal/deliverygate/gate_test.go"},
		[]string{"internal/deliverygate/gate.go (new)", "internal/deliverygate/gate_test.go (new)"},
	)
	assert.True(t, isIn, "file should match its scope entry once the (new) annotation is stripped")
	assert.Empty(t, outOfScope)
}

// TestIsWithinScope_PreservesFilenameWithLiteralParens_REQ_LNGHZN_S4_T1 verifies that a
// scope entry ending in a literal parenthesized filename (no preceding space,
// so it's not the " (new)" annotation marker) is not mistaken for an
// annotation and mis-truncated.
func TestIsWithinScope_PreservesFilenameWithLiteralParens_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/foo/bar(baz).go"},
		[]string{"internal/foo/bar(baz).go"},
	)
	assert.True(t, isIn, "filename with literal parens should match itself verbatim, not be truncated as an annotation")
	assert.Empty(t, outOfScope)
}

// TestIsWithinScope_TrailingSlashDirectoryScope_REQ_LNGHZN_S4 verifies that a
// scope entry ending in "/" (no "/**" suffix), such as "internal/", is
// treated as a recursive directory scope covering every file underneath it
// at any depth — consistent with internal/harnesspolicy/scope.go's
// cleanScope semantics for the same trailing-slash convention.
func TestIsWithinScope_TrailingSlashDirectoryScope_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/foo.go"},
		[]string{"internal/"},
	)
	assert.True(t, isIn, "file directly under a trailing-slash directory scope should be in scope")
	assert.Empty(t, outOfScope)
}

// TestIsWithinScope_TrailingSlashDirectoryScopeExcludesOutsideFiles_REQ_LNGHZN_S4
// verifies that a trailing-slash directory scope like "internal/" does not
// overly broaden matching to sibling directories outside its prefix.
func TestIsWithinScope_TrailingSlashDirectoryScopeExcludesOutsideFiles_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"other/foo.go"},
		[]string{"internal/"},
	)
	assert.False(t, isIn, "file outside the trailing-slash directory scope should not be in scope")
	assert.Equal(t, "other/foo.go", outOfScope)
}

// TestIsWithinScope_RepoRootScopeMatchesAnyFile_REQ_LNGHZN_S4_T1 verifies that the
// canonical repository-root scope "." matches any file path, consistent
// with internal/harnesspolicy/scope.go's ScopePolicy.allows, which
// special-cases scope == "." to mean "everything in the repo is in scope".
func TestIsWithinScope_RepoRootScopeMatchesAnyFile_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope([]string{"cmd/main.go"}, []string{"."})
	assert.True(t, isIn, `scope "." should cover any file path in the repo`)
	assert.Empty(t, outOfScope)
}

// TestIsWithinScope_DoublestarMidPatternMatchesAnyDepth_REQ_LNGHZN_S4_T1 verifies that a
// "**" segment appearing in the middle of a scope glob (not just as a
// trailing "/**" suffix) matches zero or more path segments, per standard
// doublestar semantics. E.g. "internal/**/api.go" should match both
// "internal/foo/api.go" (one intervening segment) and
// "internal/foo/bar/api.go" (two intervening segments), and must not match
// unrelated files in the same directories.
func TestIsWithinScope_DoublestarMidPatternMatchesAnyDepth_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	scope := []string{"internal/**/api.go"}

	isIn, outOfScope := IsWithinScope([]string{"internal/foo/api.go"}, scope)
	assert.True(t, isIn, "internal/**/api.go should match internal/foo/api.go")
	assert.Empty(t, outOfScope)

	isIn, outOfScope = IsWithinScope([]string{"internal/foo/bar/api.go"}, scope)
	assert.True(t, isIn, "internal/**/api.go should match internal/foo/bar/api.go")
	assert.Empty(t, outOfScope)

	isIn, outOfScope = IsWithinScope([]string{"internal/foo/other.go"}, scope)
	assert.False(t, isIn, "internal/**/api.go should not match internal/foo/other.go")
	assert.Equal(t, "internal/foo/other.go", outOfScope)
}
