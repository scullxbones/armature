package delivery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commitFile(t *testing.T, repo, name, body, msg string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644))
	gittest.Git(t, repo, "add", name)
	gittest.Git(t, repo, "commit", "-m", msg)
	git := adapters.New(repo)
	sha, err := git.HeadSHA()
	require.NoError(t, err)
	return sha
}

func TestRecordWritesRef_REQ_LNGHZN_S11_T1(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	base := commitFile(t, repo, "a.txt", "one\n", "init")
	gittest.Git(t, repo, "branch", "-M", "main")
	gittest.Git(t, repo, "checkout", "-b", "task/rec-01")
	tip := commitFile(t, repo, "b.txt", "two\n", "feat(rec-01): add b")

	git := adapters.New(repo)
	snap, err := Record(git, Request{
		IssueID:           "rec-01",
		IssueType:         "task",
		Base:              base,
		Tip:               tip,
		Branch:            "task/rec-01",
		IntegrationBranch: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, base, snap.Base)
	assert.Equal(t, tip, snap.Tip)
	assert.Equal(t, "task/rec-01", snap.Branch)
	assert.Equal(t, "main", snap.IntegrationBranch)

	got, err := git.ResolveRevision(RefName("rec-01"))
	require.NoError(t, err)
	assert.Equal(t, tip, got)
}

func TestValidateRejectsMissingObjects_REQ_LNGHZN_S11_T1(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	commitFile(t, repo, "a.txt", "one\n", "init")
	git := adapters.New(repo)

	_, err := Validate(git, Request{
		IssueID:   "rec-missing",
		IssueType: "task",
		Base:      "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Tip:       "cafebabecafebabecafebabecafebabecafebabe",
	})
	require.Error(t, err)
	assert.True(t, IsRecordError(err))
	var rec *RecordError
	require.ErrorAs(t, err, &rec)
	assert.Equal(t, "missing-objects", rec.Kind)
}

func TestValidateRejectsEmptyRange(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	sha := commitFile(t, repo, "a.txt", "one\n", "init")
	gittest.Git(t, repo, "branch", "task/rec-empty")
	git := adapters.New(repo)

	_, err := Validate(git, Request{
		IssueID:   "rec-empty",
		IssueType: "task",
		Base:      sha,
		Tip:       sha,
		Branch:    "task/rec-empty",
	})
	require.Error(t, err)
	var rec *RecordError
	require.ErrorAs(t, err, &rec)
	assert.Equal(t, "empty-range", rec.Kind)
}

func TestValidateRejectsUnrelatedTip(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	base := commitFile(t, repo, "a.txt", "one\n", "init")
	gittest.Git(t, repo, "branch", "-M", "main")
	gittest.Git(t, repo, "checkout", "-b", "task/rec-unrel")
	claimed := commitFile(t, repo, "b.txt", "two\n", "feat(rec-unrel): on branch")
	gittest.Git(t, repo, "checkout", "main")
	other := commitFile(t, repo, "c.txt", "other\n", "unrelated")

	git := adapters.New(repo)
	_, err := Validate(git, Request{
		IssueID:   "rec-unrel",
		IssueType: "task",
		Base:      base,
		Tip:       other,
		Branch:    "task/rec-unrel",
	})
	require.Error(t, err, "claimed tip is %s", claimed)
	var rec *RecordError
	require.ErrorAs(t, err, &rec)
	assert.Equal(t, "unrelated-range", rec.Kind)
}

func TestRecordArgvFillsIssueKeepsShaPlaceholder(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "arm delivery record --issue TASK-01 --base <sha> --tip <sha>", RecordArgv("TASK-01"))
}

func TestValidateRejectsUnreadableBoundWorktreeHEAD_REQ_LNGHZN_S11_T1(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	base := commitFile(t, repo, "a.txt", "one\n", "init")
	gittest.Git(t, repo, "branch", "-M", "main")
	gittest.Git(t, repo, "checkout", "-b", "task/rec-head")
	tip := commitFile(t, repo, "b.txt", "two\n", "feat(rec-head): add b")

	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/missing-branch\n"), 0o644))

	git := adapters.New(repo)
	_, err := Validate(git, Request{
		IssueID:      "rec-head",
		IssueType:    "task",
		Base:         base,
		Tip:          tip,
		Branch:       "task/rec-head",
		WorktreePath: repo,
	})
	require.Error(t, err, "bound worktree with unreadable HEAD must fail closed")
	var rec *RecordError
	require.ErrorAs(t, err, &rec)
	assert.Equal(t, "head-unreadable", rec.Kind)
}
