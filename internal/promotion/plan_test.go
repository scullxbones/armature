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
	"github.com/scullxbones/armature/internal/review"
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
		in := doneInput("task-ovr", base, tip, []ops.Op{
			{
				Type:      ops.OpTransition,
				TargetID:  "task-ovr",
				Timestamp: 1,
				Payload:   ops.Payload{To: ops.StatusDone, Base: base, Tip: tip},
			},
			{
				Type:      ops.OpDAGTransition,
				TargetID:  "task-ovr",
				Timestamp: 2,
				Payload: ops.Payload{
					IssueID:             "task-ovr",
					To:                  "verified",
					SkippedValidateGate: true,
					Rationale:           "post-delivery",
					Base:                base,
					Tip:                 tip,
				},
			},
		})
		res := Evaluate(git, in)
		require.True(t, res.Promote, res.Kind)
	})

	t.Run("incomplete or pre-delivery override does not waive", func(t *testing.T) {
		t.Parallel()
		git, base, tip := land(t)
		done := ops.Op{
			Type:      ops.OpTransition,
			TargetID:  "task-badovr",
			Timestamp: 2,
			Payload:   ops.Payload{To: ops.StatusDone, Base: base, Tip: tip},
		}
		complete := ops.Payload{
			IssueID:             "task-badovr",
			To:                  "verified",
			SkippedValidateGate: true,
			Rationale:           "post-delivery",
			Base:                base,
			Tip:                 tip,
		}
		cases := []struct {
			name string
			ops  []ops.Op
		}{
			{
				name: "predates delivery",
				ops: []ops.Op{
					{Type: ops.OpDAGTransition, TargetID: "task-badovr", Timestamp: 1, Payload: complete},
					done,
				},
			},
			{
				name: "not verified",
				ops: []ops.Op{
					done,
					{Type: ops.OpDAGTransition, TargetID: "task-badovr", Timestamp: 3, Payload: ops.Payload{
						IssueID: "task-badovr", To: ops.StatusDone, SkippedValidateGate: true,
						Rationale: "post-delivery", Base: base, Tip: tip,
					}},
				},
			},
			{
				name: "no rationale",
				ops: []ops.Op{
					done,
					{Type: ops.OpDAGTransition, TargetID: "task-badovr", Timestamp: 3, Payload: ops.Payload{
						IssueID: "task-badovr", To: "verified", SkippedValidateGate: true,
						Base: base, Tip: tip,
					}},
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				res := Evaluate(git, doneInput("task-badovr", base, tip, tc.ops))
				assert.False(t, res.Promote)
				assert.Equal(t, KindMissingAssessment, res.Kind)
			})
		}
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

func TestEvaluateGuardsAndFallbacks_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()

	t.Run("not done", func(t *testing.T) {
		t.Parallel()
		_, git, _ := deliveryRepo(t)
		in := doneInput("task-open", "a", "b", nil)
		in.Issue.Status = ops.StatusOpen
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindNotDone, res.Kind)
	})

	t.Run("not leaf", func(t *testing.T) {
		t.Parallel()
		_, git, _ := deliveryRepo(t)
		in := doneInput("task-parent", "a", "b", nil)
		in.Issue.Children = []string{"child"}
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindNotLeaf, res.Kind)
		var pe *Error
		require.ErrorAs(t, res.Err, &pe)
		assert.Equal(t, "arm merged --issue task-parent", pe.RecoveryArgv())
		assert.Contains(t, pe.Error(), "not a leaf")
	})

	t.Run("legacy without snapshot", func(t *testing.T) {
		t.Parallel()
		_, git, _ := deliveryRepo(t)
		in := doneInput("task-legacy", "", "", nil)
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindLegacy, res.Kind)
		assert.Contains(t, res.Recovery, "arm delivery record --issue task-legacy")
	})

	t.Run("empty commit range is empty-diff", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		gittest.Git(t, dir, "commit", "--allow-empty", "-m", "empty")
		tip := head(t, git)
		res := Evaluate(git, doneInput("task-empty-commit", base, tip, attest(t, base, tip)))
		assert.False(t, res.Promote)
		assert.Equal(t, KindEmptyDiff, res.Kind)
	})

	t.Run("reads delivery from last done payload", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		in := doneInput("task-ops", "", "", nil)
		in.Issue.Base = ""
		in.Issue.Tip = ""
		in.Issue.IntegrationBranch = ""
		in.Integration = ""
		in.PriorOps = []ops.Op{{
			Type:     ops.OpTransition,
			TargetID: "task-ops",
			Payload: ops.Payload{
				To: ops.StatusDone, Base: base, Tip: tip, IntegrationBranch: "main",
			},
		}}
		res := Evaluate(git, in)
		assert.Equal(t, KindNotOnTarget, res.Kind)
		assert.NotEmpty(t, res.CombinedPatchID)
	})

	t.Run("matching issue attestation field", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--squash", "delivery")
		gittest.Git(t, dir, "commit", "-m", "squash delivery")
		in := doneInput("task-field", base, tip, nil)
		in.Issue.AssessmentAttestations = []review.AssessmentAttestation{{
			BaseSHA: base,
			HeadSHA: tip,
			Rating:  review.Red,
		}}
		res := Evaluate(git, in)
		require.True(t, res.Promote, res.Kind)
	})

	t.Run("unreadable integration is check-failed", func(t *testing.T) {
		t.Parallel()
		dir, git, base := deliveryRepo(t)
		writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
		tip := head(t, git)
		in := doneInput("task-badint", base, tip, attest(t, base, tip))
		in.Integration = "no-such-branch"
		in.Issue.IntegrationBranch = "no-such-branch"
		res := Evaluate(git, in)
		assert.False(t, res.Promote)
		assert.Equal(t, KindCheckFailed, res.Kind)
		assert.Equal(t, "arm doctor", RecoveryArgv("task-badint", KindCheckFailed))
	})

	t.Run("rename inside a larger squash stays done", func(t *testing.T) {
		t.Parallel()
		dir := gittest.InitRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "old.txt"), []byte("body\n"), 0o644))
		gittest.Git(t, dir, "add", "old.txt")
		gittest.Git(t, dir, "commit", "-m", "init")
		gittest.Git(t, dir, "branch", "-M", "main")
		git := adapters.New(dir)
		base := head(t, git)
		gittest.Git(t, dir, "checkout", "-b", "delivery")
		require.NoError(t, os.Remove(filepath.Join(dir, "old.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("body\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "rename")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		require.NoError(t, os.Remove(filepath.Join(dir, "old.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("body\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("other\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "larger squash")
		res := Evaluate(git, doneInput("task-rename", base, tip, attest(t, base, tip)))
		assert.False(t, res.Promote)
		assert.Equal(t, KindNotOnTarget, res.Kind)
	})

	t.Run("mixed add/delete contained in larger squash promotes by tree", func(t *testing.T) {
		t.Parallel()
		dir := gittest.InitRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("remove-me\n"), 0o644))
		gittest.Git(t, dir, "add", "gone.txt")
		gittest.Git(t, dir, "commit", "-m", "init")
		gittest.Git(t, dir, "branch", "-M", "main")
		git := adapters.New(dir)
		base := head(t, git)
		gittest.Git(t, dir, "checkout", "-b", "delivery")
		require.NoError(t, os.Remove(filepath.Join(dir, "gone.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("brand-new\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "swap paths")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		require.NoError(t, os.Remove(filepath.Join(dir, "gone.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("brand-new\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("unrelated\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "larger squash with unrelated work")
		res := Evaluate(git, doneInput("task-mixed", base, tip, attest(t, base, tip)))
		require.True(t, res.Promote, "mixed add/delete that is not a rename must tree-match when contained; got %s", res.Kind)
		assert.Equal(t, KindPromote, res.Kind)
	})

	t.Run("rename with equal combined squash promotes", func(t *testing.T) {
		t.Parallel()
		dir := gittest.InitRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "old.txt"), []byte("body\n"), 0o644))
		gittest.Git(t, dir, "add", "old.txt")
		gittest.Git(t, dir, "commit", "-m", "init")
		gittest.Git(t, dir, "branch", "-M", "main")
		git := adapters.New(dir)
		base := head(t, git)
		gittest.Git(t, dir, "checkout", "-b", "delivery")
		require.NoError(t, os.Remove(filepath.Join(dir, "old.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("body\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "rename")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		gittest.Git(t, dir, "merge", "--squash", "delivery")
		gittest.Git(t, dir, "commit", "-m", "squash rename")
		res := Evaluate(git, doneInput("task-rename-eq", base, tip, attest(t, base, tip)))
		require.True(t, res.Promote, "real rename with equal combined diff must still promote; got %s", res.Kind)
		assert.Equal(t, KindPromote, res.Kind)
	})

	t.Run("rename with edits inside a larger squash stays done", func(t *testing.T) {
		t.Parallel()
		dir := gittest.InitRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "old.txt"), []byte("line one\nline two\nline three\n"), 0o644))
		gittest.Git(t, dir, "add", "old.txt")
		gittest.Git(t, dir, "commit", "-m", "init")
		gittest.Git(t, dir, "branch", "-M", "main")
		git := adapters.New(dir)
		base := head(t, git)
		gittest.Git(t, dir, "checkout", "-b", "delivery")
		require.NoError(t, os.Remove(filepath.Join(dir, "old.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("line one\nline two edited\nline three\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "rename with edit")
		tip := head(t, git)
		gittest.Git(t, dir, "checkout", "main")
		require.NoError(t, os.Remove(filepath.Join(dir, "old.txt")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("line one\nline two edited\nline three\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("unrelated\n"), 0o644))
		gittest.Git(t, dir, "add", "-A")
		gittest.Git(t, dir, "commit", "-m", "larger squash")
		res := Evaluate(git, doneInput("task-rename-edit", base, tip, attest(t, base, tip)))
		assert.False(t, res.Promote, "modified rename must not tree-match inside a larger squash; got %s", res.Kind)
		assert.Equal(t, KindNotOnTarget, res.Kind)
	})
}

func TestPostDeliveryOverrideUsesReplayOrder_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()
	dir, git, base := deliveryRepo(t)
	writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
	tip := head(t, git)
	gittest.Git(t, dir, "checkout", "main")
	gittest.Git(t, dir, "merge", "--squash", "delivery")
	gittest.Git(t, dir, "commit", "-m", "squash delivery")

	// Filename-order PriorOps: override log sorts before delivery log, but
	// timestamps put the override after the delivery transition.
	in := doneInput("task-order", base, tip, []ops.Op{
		{
			Type:      ops.OpDAGTransition,
			TargetID:  "task-order",
			Timestamp: 20,
			WorkerID:  "aaa-worker",
			Payload: ops.Payload{
				IssueID:             "task-order",
				To:                  "verified",
				SkippedValidateGate: true,
				Rationale:           "post-delivery",
				Base:                base,
				Tip:                 tip,
			},
		},
		{
			Type:      ops.OpTransition,
			TargetID:  "task-order",
			Timestamp: 10,
			WorkerID:  "zzz-worker",
			Payload:   ops.Payload{To: ops.StatusDone, Base: base, Tip: tip},
		},
	})
	res := Evaluate(git, in)
	require.True(t, res.Promote, "override after delivery in replay order must waive; got %s", res.Kind)
	assert.Equal(t, KindPromote, res.Kind)
}

func TestPostDeliveryOverrideSameSecondTieBreak_REQ_LNGHZN_S11_T2(t *testing.T) {
	t.Parallel()
	dir, git, base := deliveryRepo(t)
	writeCommit(t, dir, "feat.txt", "one\n", "feat: add feat")
	tip := head(t, git)
	gittest.Git(t, dir, "checkout", "main")
	gittest.Git(t, dir, "merge", "--squash", "delivery")
	gittest.Git(t, dir, "commit", "-m", "squash delivery")

	// Same-second cross-worker write: filename order puts the override first.
	// Replay must still treat the delivery transition as before its bound override.
	const ts int64 = 42
	in := doneInput("task-tie", base, tip, []ops.Op{
		{
			Type:      ops.OpDAGTransition,
			TargetID:  "task-tie",
			Timestamp: ts,
			WorkerID:  "aaa-worker",
			Payload: ops.Payload{
				IssueID:             "task-tie",
				To:                  "verified",
				SkippedValidateGate: true,
				Rationale:           "post-delivery",
				Base:                base,
				Tip:                 tip,
			},
		},
		{
			Type:      ops.OpTransition,
			TargetID:  "task-tie",
			Timestamp: ts,
			WorkerID:  "zzz-worker",
			Payload:   ops.Payload{To: ops.StatusDone, Base: base, Tip: tip},
		},
	})
	res := Evaluate(git, in)
	require.True(t, res.Promote, "same-second delivery+override must waive via causal tie-break; got %s", res.Kind)
	assert.Equal(t, KindPromote, res.Kind)
}

func TestPromotionErrorNilAndDefaultRecovery(t *testing.T) {
	t.Parallel()
	var e *Error
	assert.Empty(t, e.Error())
	assert.Equal(t, "arm doctor", e.RecoveryArgv())
	e = &Error{IssueID: "x", Kind: KindNotOnTarget, Msg: "nope"}
	assert.Equal(t, "nope", e.Error())
	assert.Equal(t, "arm merged --issue x", e.RecoveryArgv())
	assert.Equal(t, "arm doctor", RecoveryArgv("x", "other"))
	assert.Equal(t, "promotion check for x failed", checkMessage("x", "other"))
}
