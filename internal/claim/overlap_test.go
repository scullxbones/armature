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

	assert.True(t, globOverlaps("internal/claim/sub/*.go", "internal/claim/*.go"),
		"internal/claim/sub is genuinely nested under internal/claim and should still overlap")
	assert.True(t, globOverlaps("internal/claim/*.go", "internal/claim/sub/*.go"),
		"overlap check must be symmetric")

	assert.True(t, globOverlaps("internal/claim/a.go", "internal/claim/b.go"))
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
	{"nested dir overlap", "internal/claim/sub/*.go", "internal/claim/*.go", true},
	{"unrelated dirs", "internal/claim/a.go", "internal/validate/a.go", false},
	{"root-level files, no dir", "a.go", "b.go", false},
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
