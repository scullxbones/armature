package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/stretchr/testify/assert"
)

func TestScopesOverlap_ExcludesAncestorDescendantPairs_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

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

	scopeParent := []string{"src/**"}
	scopeChild := []string{"src/auth/**"}

	result := ScopesOverlapEx(scopeChild, scopeParent, graph, "task-01", "story-01")
	assert.False(t, result, "child task should not conflict with parent story despite scope overlap")

	result = ScopesOverlapEx(scopeParent, scopeChild, graph, "story-01", "task-01")
	assert.False(t, result, "parent story should not conflict with child task despite scope overlap")

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

func TestScopesOverlap_StillDetectsNonAncestorOverlaps_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

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

	scopeA := []string{"src/auth/**"}
	scopeB := []string{"src/auth/login.go"}

	result := ScopesOverlapEx(scopeA, scopeB, graph, "task-a", "task-b")
	assert.True(t, result, "unrelated tasks with overlapping scopes should conflict")
}

func TestGlobOverlaps_RespectsPathSegmentBoundaries_PR79(t *testing.T) {
	t.Parallel()
	assert.False(t, globOverlaps("internal/claimx/foo.go", "internal/claim/*.go"),
		"internal/claimx and internal/claim share a string prefix but are sibling directories, not nested — must not overlap")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claimx/foo.go"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/sub/*.go", "internal/claim/*.go"),
		"single-segment glob 'internal/claim/*.go' does not match the deeper literal directory 'sub/' — no longer treated as overlapping via directory ancestry")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claim/sub/*.go"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/a.go", "internal/claim/b.go"),
		"two distinct literal files that merely share a containing directory must not overlap")
}

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

func TestOverlapDetectsGlobToGlobIntersection_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/auth/*.go", "src/auth/login.*"),
		"both patterns can match src/auth/login.go and must be reported as overlapping")
	assert.True(t, globOverlaps("src/auth/login.*", "src/auth/*.go"),
		"overlap check must be symmetric")
}

func TestOverlapStripsNewFileAnnotation_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/foo.go (new)", "src/*.go"),
		"annotated new-file scope must overlap the glob that covers that file")
	assert.True(t, globOverlaps("src/*.go", "src/foo.go (new)"),
		"overlap check must be symmetric")
	assert.False(t, globOverlaps("src/foo.go (new)", "src/bar.go"),
		"annotation stripping must not invent overlap between distinct files")
}

func TestOverlapIntersectsDoublestarAcrossDirectories_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/**/foo.go", "src/auth/*.go"),
		"src/**/foo.go and src/auth/*.go both match src/auth/foo.go")
	assert.True(t, globOverlaps("src/auth/*.go", "src/**/foo.go"),
		"overlap check must be symmetric")
	assert.False(t, globOverlaps("src/**/foo.go", "src/auth/*.txt"),
		"foo.go cannot intersect a *.txt glob in the same directory")
}

func TestOverlapIntersectsCharacterClass_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("src/file[ab].go", "src/filea.*"),
		"file[ab].go and filea.* both match src/filea.go")
	assert.True(t, globOverlaps("src/filea.*", "src/file[ab].go"),
		"overlap check must be symmetric")
}

func TestOverlapGlobToGlobIntersectionBounded_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("src/auth/*.go", "src/billing/*.go"),
		"different directories can never share a matched file, regardless of filename pattern")
	assert.False(t, globOverlaps("src/billing/*.go", "src/auth/*.go"),
		"overlap check must be symmetric")
}

func TestOverlapGlobToGlobIntersectionRegression_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("docs/agents/quality-gates.md", "docs/use-cases.md"))
	assert.False(t, globOverlaps("docs/use-cases.md", "docs/agents/quality-gates.md"))

	assert.False(t, globOverlaps("internal/claim/overlap.go", "internal/claim/overlap_test.go"))
	assert.False(t, globOverlaps("internal/claim/overlap_test.go", "internal/claim/overlap.go"))
}

func TestGlobOverlapsStillMatchesIdenticalAndGlobScopes_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("README.md", "README.md"),
		"identical scope entries must still overlap")

	assert.True(t, globOverlaps("docs/agents/**", "docs/agents/quality-gates.md"),
		"an explicit directory glob must still overlap a file beneath it")
	assert.True(t, globOverlaps("docs/agents/quality-gates.md", "docs/agents/**"),
		"overlap check must be symmetric")
}

var globOverlapParityCases = []struct {
	name string
	a, b string
	want bool
}{
	{"exact match", "internal/claim/a.go", "internal/claim/a.go", true},
	{"glob vs literal in dir", "internal/claim/*.go", "internal/claim/a.go", true},
	{"sibling dir string-prefix, no overlap", "internal/claimx/foo.go", "internal/claim/*.go", false},
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

func TestIsWithinScope_FilesWithinDeclaredScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		files    []string
		scope    []string
		wantIsIn bool
		wantFile string
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

func TestIsWithinScope_StripsNewFileAnnotation_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/deliverygate/gate.go", "internal/deliverygate/gate_test.go"},
		[]string{"internal/deliverygate/gate.go (new)", "internal/deliverygate/gate_test.go (new)"},
	)
	assert.True(t, isIn, "file should match its scope entry once the (new) annotation is stripped")
	assert.Empty(t, outOfScope)
}

func TestIsWithinScope_PreservesFilenameWithLiteralParens_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/foo/bar(baz).go"},
		[]string{"internal/foo/bar(baz).go"},
	)
	assert.True(t, isIn, "filename with literal parens should match itself verbatim, not be truncated as an annotation")
	assert.Empty(t, outOfScope)
}

func TestIsWithinScope_TrailingSlashDirectoryScope_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"internal/foo.go"},
		[]string{"internal/"},
	)
	assert.True(t, isIn, "file directly under a trailing-slash directory scope should be in scope")
	assert.Empty(t, outOfScope)
}

func TestIsWithinScope_TrailingSlashDirectoryScopeExcludesOutsideFiles_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope(
		[]string{"other/foo.go"},
		[]string{"internal/"},
	)
	assert.False(t, isIn, "file outside the trailing-slash directory scope should not be in scope")
	assert.Equal(t, "other/foo.go", outOfScope)
}

func TestIsWithinScope_RepoRootScopeMatchesAnyFile_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	isIn, outOfScope := IsWithinScope([]string{"cmd/main.go"}, []string{"."})
	assert.True(t, isIn, `scope "." should cover any file path in the repo`)
	assert.Empty(t, outOfScope)
}

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
