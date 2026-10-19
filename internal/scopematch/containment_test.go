package scopematch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestIsWithinScope_CaseSensitivity_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

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
