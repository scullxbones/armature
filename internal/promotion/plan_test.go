package promotion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoteAncestorSquashAndStack_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()
	t.Run("ancestor merge commit", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--no-ff", "-m", "merge delivery", "delivery")
		in := doneInput("task-anc", base, tip, attest(t, base, tip))
		res := Evaluate(git, in)
		require.True(t, res.Promote, res.Kind)
		assert.Equal(t, KindPromote, res.Kind)
		assert.NotEmpty(t, res.TargetSHA)
		assert.NotEmpty(t, res.MatchedCommit)
		assert.NotEmpty(t, res.CombinedPatchID)
		assert.Equal(t, res.MatchedCommit, res.MergedPayload.MatchedCommit)
	})

	t.Run("squash of one range", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--squash", "delivery")
		gittest.Git(t, dir, "commit", "-m", "squash delivery")
		res := Evaluate(git, doneInput("task-sq", base, tip, attest(t, base, tip)))
		require.True(t, res.Promote, res.Kind)
		assert.Equal(t, KindPromote, res.Kind)
		assert.NotEmpty(t, res.MatchedCommit)
	})

	t.Run("stack squash whose blobs contain the range", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "a.txt", "a1\n", "feat: a")
		writeCommit(t, dir, "b.txt", "b1\n", "feat: b")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--squash", "delivery")
		gittest.Git(t, dir, "commit", "-m", "squash stack")
		res := Evaluate(git, doneInput("task-stack", base, tip, attest(t, base, tip)))
		require.True(t, res.Promote, res.Kind)
		assert.Equal(t, KindPromote, res.Kind)
	})
}

func TestPromoteRequiresMatchingAssessment_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()
	land := func(t *testing.T) (git *adapters.Client, base, tip string) {
		t.Helper()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip = head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--squash", "delivery")
		gittest.Git(t, dir, "commit", "-m", "squash delivery")
		return git, base, tip
	}

	t.Run("matching attestation even when red", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		res := Evaluate(git, doneInput("task-att", base, tip, attest(t, base, tip)))
		require.True(t, res.Promote, res.Kind)
	})

	t.Run("missing assessment stays done", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		res := Evaluate(git, doneInput("task-noatt", base, tip, nil))
		assert.False(t, res.Promote)
		assert.Equal(t, KindMissingAssessment, res.Kind)
		assert.True(t, res.AppendCheck)
		assert.Contains(t, res.Recovery, "arm review record --issue task-noatt")
	})

	t.Run("plan-time override-release does not waive", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		in := doneInput("task-plan", base, tip, []ops.Op{{
			Type:     ops.OpDAGTransition,
			TargetID: "task-plan",
			Payload: ops.Payload{
				IssueID:             "task-plan",
				To:                  "verified",
				SkippedValidateGate: true,
				Rationale:           "plan-time",
			},
		}})
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindMissingAssessment, res.Kind)
	})

	t.Run("ordinary decision does not waive", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		in := doneInput("task-dec", base, tip, []ops.Op{{
			Type:     ops.OpDecision,
			TargetID: "task-dec",
			Payload: ops.Payload{
				Topic:     "missing-assessment",
				Choice:    "waive",
				Rationale: "ship it",
			},
		}})
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindMissingAssessment, res.Kind)
	})

	t.Run("post-delivery override bound to base and tip", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		in := doneInput("task-ovr", base, tip, []ops.Op{{
			Type:     ops.OpDAGTransition,
			TargetID: "task-ovr",
			Payload: ops.Payload{
				IssueID:             "task-ovr",
				To:                  "verified",
				SkippedValidateGate: true,
				Rationale:           "post-delivery",
				Base:                base,
				Tip:                 tip,
			},
		}})
		res := Evaluate(git, in)
		require.True(t, res.Promote, res.Kind)
	})
}

func TestPartialCherryPickAndEmptyDiffStayDone_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()
	t.Run("partial cherry-pick", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		first := writeCommit(t, dir, "a.txt", "a\n", "feat: a")
		writeCommit(t, dir, "b.txt", "b\n", "feat: b")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "cherry-pick", first)
		res := Evaluate(git, doneInput("task-cherry", base, tip, attest(t, base, tip)))
		assert.False(t, res.Promote)
		assert.Equal(t, KindNotOnTarget, res.Kind)
		assert.True(t, res.AppendCheck)
	})

	t.Run("empty diff", func(t *testing.T) {
		t.Parallel()
		_, git, _ := deliveryRepo(t)
		sha := head(t, git)
		res := Evaluate(git, doneInput("task-empty", sha, sha, attest(t, sha, sha)))
		assert.False(t, res.Promote)
		assert.Equal(t, KindEmptyDiff, res.Kind)
	})

	t.Run("whitespace-only squash is not a patch-id-only match", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "feat.txt"), []byte("one \n"), 0o644))
		gittest.Git(t, dir, "add", "feat.txt")
		gittest.Git(t, dir, "commit", "-m", "whitespace only")
		res := Evaluate(git, doneInput("task-ws", base, tip, attest(t, base, tip)))
		assert.False(t, res.Promote)
		assert.Equal(t, KindNotOnTarget, res.Kind)
	})
}

func TestEvaluateDedupsPromotionCheck(t *testing.T) {
	t.Parallel()
	dir, git, base := deliveryRepo(t)
	writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
	tip := head(t, git)
	in := doneInput("task-dedup", base, tip, nil)
	first := Evaluate(git, in)
	require.Equal(t, KindNotOnTarget, first.Kind)
	require.True(t, first.AppendCheck)
	in.PriorOps = append(in.PriorOps, ops.Op{
		Type:     ops.OpPromotionCheck,
		TargetID: "task-dedup",
		Payload:  first.CheckPayload,
	})
	second := Evaluate(git, in)
	assert.Equal(t, KindNotOnTarget, second.Kind)
	assert.False(t, second.AppendCheck)
}

func deliveryRepo(t *testing.T) (dir string, git *adapters.Client, base string) {
	t.Helper()
	dir = gittest.InitRepo(t)
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, dir, "branch", "-M", "main")
	git = adapters.New(dir)
	base = head(t, git)
	gittest.Git(t, dir, "checkout", "-b", "delivery")
	return dir, git, base
}

func writeCommit(t *testing.T, dir, rel, content, msg string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	gittest.Git(t, dir, "add", rel)
	gittest.Git(t, dir, "commit", "-m", msg)
	return head(t, adapters.New(dir))
}

func head(t *testing.T, git *adapters.Client) string {
	t.Helper()
	sha, err := git.HeadSHA()
	require.NoError(t, err)
	return sha
}

func doneInput(id, base, tip string, extra []ops.Op) Input {
	issue := materialize.Issue{
		ID:                id,
		Status:            ops.StatusDone,
		Base:              base,
		Tip:               tip,
		IntegrationBranch: "main",
		Children:          []string{},
	}
	opsCopy := append([]ops.Op(nil), extra...)
	for i := range opsCopy {
		opsCopy[i].TargetID = id
	}
	return Input{Issue: issue, PriorOps: opsCopy, Integration: "main"}
}

func attest(t *testing.T, base, tip string) []ops.Op {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"base_sha": base,
		"head_sha": tip,
		"rating":   "red",
	})
	require.NoError(t, err)
	return []ops.Op{{
		Type:     ops.OpAssessmentAttested,
		TargetID: "ignored",
		Payload:  ops.Payload{Assessment: body},
	}}
}
