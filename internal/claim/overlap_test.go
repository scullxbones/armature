package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/scopematch"
	"github.com/stretchr/testify/assert"
)

func globOverlaps(a, b string) bool {
	ok, err := scopematch.Overlaps(a, b)
	if err != nil {
		panic(err)
	}
	return ok
}

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

	result := scopesOverlapIgnoringAncestry(scopeChild, scopeParent, graph, "task-01", "story-01")
	assert.False(t, result, "child task should not conflict with parent story despite scope overlap")

	result = scopesOverlapIgnoringAncestry(scopeParent, scopeChild, graph, "story-01", "task-01")
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

	result = scopesOverlapIgnoringAncestry(scopeChild, scopeChild, graph, "task-01", "task-02")
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

	result := scopesOverlapIgnoringAncestry(scopeA, scopeB, graph, "task-a", "task-b")
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

func TestGlobOverlaps_ParityWithValidatePackage_PR79(t *testing.T) {
	t.Parallel()
	for _, c := range overlapParityCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, globOverlaps(c.a, c.b), "globOverlaps(%q, %q)", c.a, c.b)
			assert.Equal(t, c.want, globOverlaps(c.b, c.a), "globOverlaps(%q, %q) (symmetric)", c.b, c.a)
		})
	}
}

func overlapParityCases() []struct {
	name string
	a, b string
	want bool
} {
	return []struct {
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
		{"glob-vs-glob same-segment intersection", "src/auth/*.go", "src/auth/login.*", true},
		{"glob-vs-glob distinct literal directories cannot intersect", "src/auth/*.go", "src/billing/*.go", false},
	}
}
