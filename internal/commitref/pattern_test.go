package commitref_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/scullxbones/armature/internal/commitref"
)

// TestTypedCommitPattern_REQ_LNGHZN_S4 verifies the shared typed
// conventional-commit pattern matches every documented type from
// docs/conventions.md, matches the breaking-change bang form, rejects a
// disallowed type, and requires a non-empty description after the colon.
func TestTypedCommitPattern_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	for _, typ := range commitref.CommitTypes {
		pattern := commitref.TypedCommitPattern("ISSUE-1")
		assert.True(t, pattern.MatchString(typ+"(ISSUE-1): description"), "type %q should match", typ)
	}

	pattern := commitref.TypedCommitPattern("ISSUE-1")
	assert.True(t, pattern.MatchString("feat(ISSUE-1)!: breaking change"), "breaking-change bang form should match")
	assert.False(t, pattern.MatchString("oops(ISSUE-1): bypass convention"), "disallowed type should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-1):"), "bare subject with no description should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-2): description"), "different issue ID should not match")
}

// TestMergeCommitPattern_REQ_LNGHZN_S4 verifies the shared merge-commit
// reference pattern accepts the documented "merge: ISSUE-ID description"
// form, requires a non-empty description, and does not match an unrelated
// issue ID.
func TestMergeCommitPattern_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	pattern := commitref.MergeCommitPattern("ISSUE-1")
	assert.True(t, pattern.MatchString("merge: ISSUE-1 integrate feature work"))
	assert.False(t, pattern.MatchString("merge: ISSUE-1"), "bare subject with no description should not match")
	assert.False(t, pattern.MatchString("merge: ISSUE-2 integrate feature work"), "different issue ID should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-1): description"), "typed form should not match the merge pattern")
}
