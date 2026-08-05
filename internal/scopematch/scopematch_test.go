package scopematch

import "testing"

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
