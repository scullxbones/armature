package scopematch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopesOverlap_ListPairs(t *testing.T) {
	t.Parallel()
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}

func TestScopesOverlap_MalformedPatternFailOpen_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	_, err := Overlaps("foo[", "foo.go")
	require.Error(t, err, "helper must surface the malformed pattern")
	assert.False(t, ScopesOverlap([]string{"foo["}, []string{"foo.go"}),
		"malformed patterns fail open so Ready waves and Claim planning stay unchanged")
	assert.False(t, ScopesOverlap([]string{"foo.go"}, []string{"foo["}))
	assert.False(t, ScopesOverlap([]string{"*"}, []string{"["}),
		"a valid glob matching the malformed literal must still fail open")
	assert.False(t, ScopesOverlap([]string{"["}, []string{"*"}),
		"fail-open must not depend on argument order")
	assert.True(t, ScopesOverlap([]string{"src/a.go"}, []string{"src/a.go"}),
		"valid overlapping scopes must still overlap")
}

func TestOverlapParityCases_REQ_PR79(t *testing.T) {
	t.Parallel()
	for _, c := range overlapParityCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, mustOverlaps(t, c.a, c.b), "Overlaps(%q, %q)", c.a, c.b)
			assert.Equal(t, c.want, mustOverlaps(t, c.b, c.a), "Overlaps(%q, %q) (symmetric)", c.b, c.a)
		})
	}
}
