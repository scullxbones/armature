package commitref_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/scullxbones/armature/internal/commitref"
)

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

func TestMergeCommitPattern_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	pattern := commitref.MergeCommitPattern("ISSUE-1")
	assert.True(t, pattern.MatchString("merge: ISSUE-1 integrate feature work"))
	assert.False(t, pattern.MatchString("merge: ISSUE-1"), "bare subject with no description should not match")
	assert.False(t, pattern.MatchString("merge: ISSUE-2 integrate feature work"), "different issue ID should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-1): description"), "typed form should not match the merge pattern")
}

func TestSubjectHasIssueScope(t *testing.T) {
	t.Parallel()

	for _, typ := range commitref.CommitTypes {
		assert.True(t, commitref.SubjectHasIssueScope(typ+"(ISSUE-1): description", 1), "type %q should match", typ)
	}

	assert.True(t, commitref.SubjectHasIssueScope("fix(D12): surface fetch failure", 1))
	assert.True(t, commitref.SubjectHasIssueScope("refactor(GNHF): shrink validate parse helpers", 1))
	assert.True(t, commitref.SubjectHasIssueScope("ci(TOPTIER-S6): fan out check jobs", 1))
	assert.True(t, commitref.SubjectHasIssueScope("feat(ISSUE-1)!: breaking change", 1))
	assert.True(t, commitref.SubjectHasIssueScope("fix(D12): surface fetch failure (#271)", 1), "GitHub squash suffix should be stripped")
	assert.True(t, commitref.SubjectHasIssueScope("merge: GNHF integrate nightly", 2))

	assert.False(t, commitref.SubjectHasIssueScope("refactor: overnight simplify production Go (no behavior change)", 1))
	assert.False(t, commitref.SubjectHasIssueScope("fix: no scope", 1))
	assert.False(t, commitref.SubjectHasIssueScope("feat(PARCLI) 2/3: thread Env", 1), "space between scope and colon should not match")
	assert.False(t, commitref.SubjectHasIssueScope("chore(AGTFND): ignore me", 1), "chore is not a documented type")
	assert.False(t, commitref.SubjectHasIssueScope("ci: report per-package test timings", 1))
	assert.False(t, commitref.SubjectHasIssueScope("merge: GNHF integrate nightly", 1), "single-parent merge form should not match")
	assert.False(t, commitref.SubjectHasIssueScope("feat(ISSUE-1):", 1), "bare subject with no description should not match")
	assert.False(t, commitref.SubjectHasIssueScope("oops(ISSUE-1): bypass convention", 1))
}
