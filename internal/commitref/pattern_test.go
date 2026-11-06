package commitref

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypedCommitPattern_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	for _, typ := range commitTypes() {
		pattern := typedCommitPattern("ISSUE-1")
		assert.True(t, pattern.MatchString(typ+"(ISSUE-1): description"), "type %q should match", typ)
	}

	pattern := typedCommitPattern("ISSUE-1")
	assert.True(t, pattern.MatchString("feat(ISSUE-1)!: breaking change"), "breaking-change bang form should match")
	assert.False(t, pattern.MatchString("oops(ISSUE-1): bypass convention"), "disallowed type should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-1):"), "bare subject with no description should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-2): description"), "different issue ID should not match")
}

func TestMergeCommitPattern_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	pattern := mergeCommitPattern("ISSUE-1")
	assert.True(t, pattern.MatchString("merge: ISSUE-1 integrate feature work"))
	assert.False(t, pattern.MatchString("merge: ISSUE-1"), "bare subject with no description should not match")
	assert.False(t, pattern.MatchString("merge: ISSUE-2 integrate feature work"), "different issue ID should not match")
	assert.False(t, pattern.MatchString("feat(ISSUE-1): description"), "typed form should not match the merge pattern")
}

func TestIsValidReference_QuotesIssueIDLiterals(t *testing.T) {
	t.Parallel()

	id := "ISSUE.1+2"
	assert.True(t, IsValidReference("feat(ISSUE.1+2): description", 1, id))
	assert.False(t, IsValidReference("feat(ISSUEX1+2): description", 1, id), "dot must stay literal")
	assert.False(t, IsValidReference("feat(ISSUE.112): description", 1, id), "plus must stay literal")
	assert.True(t, IsValidReference("merge: ISSUE.1+2 integrate", 2, id))
	assert.False(t, IsValidReference("merge: ISSUEX1+2 integrate", 2, id))
	assert.False(t, IsValidReference("merge: ISSUE.1+2 integrate", 1, id), "single-parent merge form should not match")

	spaced := "foo bar"
	assert.True(t, IsValidReference("feat(foo bar): description", 1, spaced))
	assert.True(t, IsValidReference("merge: foo bar integrate", 2, spaced))
	assert.False(t, IsValidReference("merge: foo integrate", 2, spaced))
}

func TestSubjectHasIssueScope(t *testing.T) {
	t.Parallel()

	for _, typ := range commitTypes() {
		assert.True(t, subjectHasIssueScope(typ+"(ISSUE-1): description", 1), "type %q should match", typ)
	}

	assert.True(t, subjectHasIssueScope("fix(D12): surface fetch failure", 1))
	assert.True(t, subjectHasIssueScope("refactor(GNHF): shrink validate parse helpers", 1))
	assert.True(t, subjectHasIssueScope("ci(TOPTIER-S6): fan out check jobs", 1))
	assert.True(t, subjectHasIssueScope("feat(ISSUE-1)!: breaking change", 1))
	assert.True(t, subjectHasIssueScope("fix(D12): surface fetch failure (#271)", 1), "GitHub squash suffix should be stripped")
	assert.True(t, subjectHasIssueScope("merge: GNHF integrate nightly", 2))

	assert.False(t, subjectHasIssueScope("refactor: overnight simplify production Go (no behavior change)", 1))
	assert.False(t, subjectHasIssueScope("fix: no scope", 1))
	assert.False(t, subjectHasIssueScope("feat(PARCLI) 2/3: thread Env", 1), "space between scope and colon should not match")
	assert.False(t, subjectHasIssueScope("chore(AGTFND): ignore me", 1), "chore is not a documented type")
	assert.False(t, subjectHasIssueScope("ci: report per-package test timings", 1))
	assert.False(t, subjectHasIssueScope("merge: GNHF integrate nightly", 1), "single-parent merge form should not match")
	assert.False(t, subjectHasIssueScope("feat(ISSUE-1):", 1), "bare subject with no description should not match")
	assert.False(t, subjectHasIssueScope("oops(ISSUE-1): bypass convention", 1))
}
