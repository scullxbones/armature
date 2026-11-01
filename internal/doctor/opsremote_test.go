package doctor

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/testexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeD11Git injects typed probe outcomes for fail-closed D11 tables.
type fakeD11Git struct {
	fetchErr            error
	tip                 string
	resolveErr          error
	ahead               int
	revListErr          error
	committerUnixes     []int64
	committerRangeErr   error
	committerRangeCalls int
	deleted             []string
}

func (g *fakeD11Git) FetchBranchToRef(string, string) error { return g.fetchErr }

func (g *fakeD11Git) ResolveRevision(rev string) (string, error) {
	if !strings.HasPrefix(rev, d11RunNS+"/") {
		return "", fmt.Errorf("fakeD11Git: unexpected rev %q", rev)
	}
	if g.resolveErr != nil {
		return "", g.resolveErr
	}
	return g.tip, nil
}

func (g *fakeD11Git) RevListCount(string) (int, error) {
	if g.revListErr != nil {
		return 0, g.revListErr
	}
	return g.ahead, nil
}

func (g *fakeD11Git) CommitterUnixRange(string) ([]int64, error) {
	g.committerRangeCalls++
	if g.committerRangeErr != nil {
		return nil, g.committerRangeErr
	}
	out := make([]int64, len(g.committerUnixes))
	copy(out, g.committerUnixes)
	return out, nil
}

func (g *fakeD11Git) DeleteRef(ref string) error {
	g.deleted = append(g.deleted, ref)
	return nil
}

func TestObserveEvaluateD11_FailClosedTable_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	staleAfter := 24 * time.Hour
	freshUnix := now.Add(-time.Hour).Unix()
	staleUnix := now.Add(-(staleAfter + time.Hour)).Unix()
	missingFetch := fmt.Errorf("%w: fatal: couldn't find remote ref refs/heads/_armature", adapters.ErrRemoteRefMissing)
	unreachable := errors.New("fatal: unable to access 'origin': Could not resolve host")

	tests := []struct {
		name       string
		g          *fakeD11Git
		staleAfter time.Duration
		wantOut    d11Outcome
		wantSev    Severity
		wantExit   int
		contains   string
		itemsHint  bool
	}{
		{
			name:     "healthy_in_sync",
			g:        &fakeD11Git{tip: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			wantOut:  d11Healthy,
			wantSev:  SeverityOK,
			wantExit: 0,
			contains: "verified",
		},
		{
			name: "healthy_fresh_local_only",
			g: &fakeD11Git{
				tip:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:           2,
				committerUnixes: []int64{freshUnix, freshUnix},
			},
			wantOut:  d11Healthy,
			wantSev:  SeverityOK,
			wantExit: 0,
			contains: "within stale threshold",
		},
		{
			name: "stale_local_only",
			g: &fakeD11Git{
				tip:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:           3,
				committerUnixes: []int64{staleUnix, staleUnix, staleUnix},
			},
			wantOut:   d11Stale,
			wantSev:   SeverityWarning,
			wantExit:  0,
			contains:  "Local-only ops",
			itemsHint: true,
		},
		{
			name: "stale_threshold_override_short",
			g: &fakeD11Git{
				tip:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:           1,
				committerUnixes: []int64{now.Add(-2 * time.Hour).Unix()},
			},
			staleAfter: time.Hour,
			wantOut:    d11Stale,
			wantSev:    SeverityWarning,
			wantExit:   0,
			contains:   "Local-only ops",
			itemsHint:  true,
		},
		{
			name:      "missing_positive_evidence",
			g:         &fakeD11Git{fetchErr: missingFetch},
			wantOut:   d11Missing,
			wantSev:   SeverityWarning,
			wantExit:  0,
			contains:  "no verified remote tracking tip",
			itemsHint: true,
		},
		{
			name:     "unreachable_no_tracking_could_not_verify",
			g:        &fakeD11Git{fetchErr: unreachable},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name: "unreachable_with_old_tip_still_could_not_verify",
			// tip is intentionally set: a buggy observer might resolve stale
			// shared tracking. Fail-closed must not consult it after fetch fail.
			g:        &fakeD11Git{fetchErr: unreachable, tip: "staleaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name:     "fetch_ok_resolve_fails",
			g:        &fakeD11Git{resolveErr: errors.New("resolve boom")},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name:     "fetch_ok_empty_tip",
			g:        &fakeD11Git{tip: ""},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name: "rev_list_fails",
			g: &fakeD11Git{
				tip:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				revListErr: errors.New("rev-list boom"),
			},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name: "committer_range_fails",
			g: &fakeD11Git{
				tip:               "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:             2,
				committerRangeErr: errors.New("committer range boom"),
			},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name: "count_mismatch_fail_closed",
			g: &fakeD11Git{
				tip:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:           3,
				committerUnixes: []int64{freshUnix, freshUnix},
			},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
		{
			name: "merged_side_branch_ages_from_oldest_not_merge",
			// Count includes the old side-branch tip; first-parent would only
			// see the recent merge. Full-range oldest must still be stale.
			g: &fakeD11Git{
				tip:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:           2,
				committerUnixes: []int64{freshUnix, staleUnix},
			},
			wantOut:   d11Stale,
			wantSev:   SeverityWarning,
			wantExit:  0,
			contains:  "Local-only ops",
			itemsHint: true,
		},
		{
			name: "committer_time_fails",
			g: &fakeD11Git{
				tip:               "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ahead:             1,
				committerRangeErr: errors.New("committer boom"),
			},
			wantOut:  d11CouldNotVerify,
			wantSev:  SeverityError,
			wantExit: 1,
			contains: "Could not verify",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			threshold := tc.staleAfter
			if threshold == 0 {
				threshold = staleAfter
			}
			obs := observeD11OpsRemote(tc.g, now, threshold)
			assert.Equal(t, tc.wantOut, obs.outcome)
			require.NotEmpty(t, tc.g.deleted, "per-run ref must be cleaned up")
			assert.True(t, strings.HasPrefix(tc.g.deleted[0], d11RunNS+"/"))

			got := evaluateD11OpsRemote(obs, "")
			assert.Equal(t, "D11", got.Check)
			assert.Equal(t, tc.wantSev, got.Severity)
			assert.Contains(t, got.Message, tc.contains)
			assert.Equal(t, tc.wantExit, d11DoctorExit(got, false))
			if tc.wantSev == SeverityWarning {
				assert.Equal(t, 1, d11DoctorExit(got, true), "--strict promotes warnings")
			}
			if tc.itemsHint {
				require.Equal(t, []string{d11RemediationHint}, got.Items)
				assert.Equal(t, "arm push-ops", got.Items[0])
				assert.NotContains(t, got.Items[0], "ops-backup")
			}
			assert.NotContains(t, got.Message, "docs/ops-backup.md")
		})
	}
}

func TestEvaluateD11OpsRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		obs      d11Observation
		severity Severity
		contains string
		items    []string
		exit     int
	}{
		{
			name:     "verified ok in sync",
			obs:      d11Observation{outcome: d11Healthy},
			severity: SeverityOK,
			contains: "verified",
			exit:     0,
		},
		{
			name: "fresh local-only names threshold check",
			obs: d11Observation{
				outcome:        d11Healthy,
				localOnlyCount: 2,
			},
			severity: SeverityOK,
			contains: "within stale threshold",
			exit:     0,
		},
		{
			name:     "missing tracking warns",
			obs:      d11Observation{outcome: d11Missing},
			severity: SeverityWarning,
			contains: "no verified remote tracking tip",
			items:    []string{d11RemediationHint},
			exit:     0,
		},
		{
			name: "stale local-only warns",
			obs: d11Observation{
				outcome:        d11Stale,
				localOnlyCount: 3,
				localOnlyAge:   defaultLocalOnlyOpsStaleAfter + time.Minute,
			},
			severity: SeverityWarning,
			contains: "Local-only ops",
			items:    []string{d11RemediationHint},
			exit:     0,
		},
		{
			name:     "could not verify errors",
			obs:      d11Observation{outcome: d11CouldNotVerify, detail: "network down"},
			severity: SeverityError,
			contains: "Could not verify",
			items:    []string{"network down"},
			exit:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evaluateD11OpsRemote(tc.obs, "")
			assert.Equal(t, "D11", got.Check)
			assert.Equal(t, tc.severity, got.Severity)
			assert.Contains(t, got.Message, tc.contains)
			assert.Equal(t, tc.items, got.Items)
			assert.Equal(t, tc.exit, d11DoctorExit(got, false))
			assert.NotContains(t, got.Message, "docs/ops-backup.md")
			for _, item := range got.Items {
				assert.NotContains(t, item, "docs/ops-backup.md")
				if tc.severity == SeverityWarning {
					assert.Contains(t, item, "arm push-ops")
				}
			}
		})
	}
}

func TestEvaluateD11OpsRemote_ConfigFallbackWarn_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	warn := "could not load local_only_ops_stale_hours; using default 24h: parse boom"

	healthy := evaluateD11OpsRemote(d11Observation{outcome: d11Healthy}, warn)
	assert.Equal(t, SeverityWarning, healthy.Severity)
	assert.Contains(t, healthy.Message, "using default 24h")

	missing := evaluateD11OpsRemote(d11Observation{outcome: d11Missing}, warn)
	assert.Equal(t, SeverityWarning, missing.Severity)
	assert.Contains(t, missing.Message, "no verified remote tracking tip")
	require.GreaterOrEqual(t, len(missing.Items), 2)
	assert.Contains(t, missing.Items[0], "using default 24h")
	assert.Equal(t, d11RemediationHint, missing.Items[1])
}

func TestDoctorD11_WarnsOnMissingRemoteTracking_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	// Reachable origin without refs/heads/_armature → positive missing evidence.
	// A repo with no origin is could-not-verify (transport), not missing.
	fx := gittest.InitWithOrigin(t)
	worktree := fx.Dir
	runGit(t, worktree, "checkout", "-b", "_armature")
	runGit(t, worktree, "commit", "--allow-empty", "-m", "local-only ops")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityWarning, d11.Severity)
	assert.Contains(t, d11.Message, "no verified remote tracking tip")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
	assert.Equal(t, 0, d11DoctorExit(d11, false))
	assert.NotContains(t, d11.Message, "docs/ops-backup.md")
}

func TestDoctorD11_WarnsOnStaleLocalOnlyOps_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	staleWhen := time.Now().Add(-(defaultLocalOnlyOpsStaleAfter + 2*time.Hour))
	runGitEnv(t, worktree, map[string]string{
		"GIT_AUTHOR_DATE":    staleWhen.Format(time.RFC3339),
		"GIT_COMMITTER_DATE": staleWhen.Format(time.RFC3339),
	}, "commit", "--allow-empty", "-m", "unpublished local ops")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityWarning, d11.Severity)
	assert.Contains(t, d11.Message, "Local-only ops")
	assert.Contains(t, d11.Message, "unpublished")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
	assert.Equal(t, 0, d11DoctorExit(d11, false))
	assert.NotContains(t, d11.Message, "docs/ops-backup.md")
}

func TestDoctorD11_AgesMergedSideBranchNotJustFirstParent_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	// Codex P2 repro: side branch committer backdated 30h, --no-ff merge onto
	// _armature with a fresh merge commit while origin stays at the merge base.
	// First-parent age would be healthy; full remote..HEAD oldest must warn.
	worktree, _ := opsWorktreeWithOrigin(t)
	base := gitOut(t, worktree, "rev-parse", "HEAD")

	staleWhen := time.Now().Add(-30 * time.Hour)
	runGit(t, worktree, "checkout", "-b", "side-old")
	runGitEnv(t, worktree, map[string]string{
		"GIT_AUTHOR_DATE":    staleWhen.Format(time.RFC3339),
		"GIT_COMMITTER_DATE": staleWhen.Format(time.RFC3339),
	}, "commit", "--allow-empty", "-m", "old unpublished side branch")

	runGit(t, worktree, "checkout", "_armature")
	runGit(t, worktree, "merge", "--no-ff", "-m", "recent merge of old side", "side-old")
	require.NotEqual(t, base, gitOut(t, worktree, "rev-parse", "HEAD"))

	d11 := checkD11OpsRemote(worktree, "", time.Now())
	assert.Equal(t, SeverityWarning, d11.Severity, d11.Message)
	assert.Contains(t, d11.Message, "Local-only ops")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
}

// TestDoctorD11_AgeGitInvocationsBounded_REQ_TOPTIER_S12_T2 proves D11 ages
// unpublished commits with a constant number of git processes (one batched
// CommitterUnixRange), not one git log -1 per SHA. Codex P2 repro shape:
// commit-tree chain with a backdated first commit, then count execs.
func TestDoctorD11_AgeGitInvocationsBounded_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	countFor := func(n int) (calls int, obs d11Observation) {
		t.Helper()
		worktree, _ := opsWorktreeWithOrigin(t)
		chainUnpublishedCommits(t, worktree, n, time.Now().Add(-30*time.Hour))

		counterPath := filepath.Join(t.TempDir(), "git-calls")
		require.NoError(t, os.WriteFile(counterPath, nil, 0o644))
		env := countingGitEnviron(t, counterPath)
		gc := adapters.New(worktree).WithEnviron(env)

		obs = observeD11OpsRemote(gc, time.Now(), defaultLocalOnlyOpsStaleAfter)
		raw, err := os.ReadFile(counterPath)
		require.NoError(t, err)
		return len(raw), obs
	}

	smallCalls, smallObs := countFor(40)
	largeCalls, largeObs := countFor(200)
	assert.Equal(t, d11Stale, smallObs.outcome, smallObs.detail)
	assert.Equal(t, d11Stale, largeObs.outcome, largeObs.detail)
	assert.Equal(t, 40, smallObs.localOnlyCount)
	assert.Equal(t, 200, largeObs.localOnlyCount)
	assert.Equal(t, smallCalls, largeCalls,
		"git exec count must be independent of unpublished commit count (got %d for 40, %d for 200)",
		smallCalls, largeCalls)
	// fetch + rev-parse tip + rev-list --count + log --format=%ct + delete-ref
	assert.LessOrEqual(t, largeCalls, 8, "D11 probe must stay a small constant git budget")
	assert.GreaterOrEqual(t, largeCalls, 4)
}

func countingGitEnviron(t *testing.T, counterPath string) []string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(realGit))

	wrapperDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf x >> %q\nexec %q \"$@\"\n", counterPath, realGit)
	testexec.Write(t, filepath.Join(wrapperDir, "git"), []byte(script))

	env := append([]string{}, os.Environ()...)
	pathSet := false
	overlay := wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH")
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") || strings.HasPrefix(e, "Path=") {
			env[i] = "PATH=" + overlay
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH="+overlay)
	}
	return env
}

// chainUnpublishedCommits appends n empty commits via commit-tree (fast),
// backdating the first committer timestamp to staleWhen. Origin tip stays put.
func chainUnpublishedCommits(t *testing.T, worktree string, n int, staleWhen time.Time) {
	t.Helper()
	require.Greater(t, n, 0)
	tree := gitOut(t, worktree, "write-tree")
	parent := gitOut(t, worktree, "rev-parse", "HEAD")
	for i := 0; i < n; i++ {
		env := map[string]string{}
		if i == 0 {
			stamp := staleWhen.Format(time.RFC3339)
			env["GIT_AUTHOR_DATE"] = stamp
			env["GIT_COMMITTER_DATE"] = stamp
		}
		parent = gitOutEnv(t, worktree, env, "commit-tree", tree, "-p", parent, "-m", fmt.Sprintf("unpublished-%d", i))
	}
	runGit(t, worktree, "reset", "--hard", parent)
}

func gitOutEnv(t *testing.T, dir string, env map[string]string, args ...string) string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	cmd.Env = append(os.Environ(), cmd.Env...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func TestDoctorD11_ConfigStaleHoursOverride_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	// 2h old local-only — under default 24h, over configured 1h.
	when := time.Now().Add(-2 * time.Hour)
	runGitEnv(t, worktree, map[string]string{
		"GIT_AUTHOR_DATE":    when.Format(time.RFC3339),
		"GIT_COMMITTER_DATE": when.Format(time.RFC3339),
	}, "commit", "--allow-empty", "-m", "config-threshold unpublished")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "config.json"), []byte(`{
		"project_type": "go",
		"default_ttl": 60,
		"token_budget": 1600,
		"low_stakes_push_threshold": 5,
		"local_only_ops_stale_hours": 1,
		"hooks": []
	}`), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityWarning, d11.Severity)
	assert.Contains(t, d11.Message, "Local-only ops")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
}

func TestRun_D11_OKWhenInSync(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Contains(t, d11.Message, "verified")
	assert.Empty(t, d11.Items)
	assert.Equal(t, 0, d11DoctorExit(d11, false))
}

func TestRun_D11_SkipWhenWorktreeMissing(t *testing.T) {
	t.Parallel()

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Equal(t, "Ops remote tracking not checked", d11.Message)
}

func TestRun_D11_FreshLocalOnlyUnderThresholdOK(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	runGit(t, worktree, "commit", "--allow-empty", "-m", "fresh unpublished")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Contains(t, d11.Message, "within stale threshold")
	assert.NotContains(t, d11.Message, "tracking tip is verified")
}

func TestDoctorD11_DoesNotMutateSharedTracking_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree, originClone := opsWorktreeWithOrigin(t)
	before := gitOut(t, worktree, "rev-parse", "refs/remotes/origin/_armature")

	// Advance origin tip without updating this clone's shared tracking, then
	// run D11 alone (not full Run — D12 refreshes shared tracking by design).
	runGit(t, originClone, "commit", "--allow-empty", "-m", "origin advanced while tracking stale")
	runGit(t, originClone, "push", "origin", "_armature")
	advanced := gitOut(t, originClone, "rev-parse", "HEAD")
	require.NotEqual(t, before, advanced)

	d11 := checkD11OpsRemote(worktree, "", time.Now())
	assert.Equal(t, SeverityOK, d11.Severity, "fresh evidence should verify against advanced tip")
	after := gitOut(t, worktree, "rev-parse", "refs/remotes/origin/_armature")
	assert.Equal(t, before, after, "D11 must not refresh shared refs/remotes/origin/_armature")
	assert.Empty(t, listRefs(t, worktree, d11RunNS), "per-run D11 refs cleaned up")
}

func TestDoctorD11_UnreachableOriginCouldNotVerify_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	before := gitOut(t, worktree, "rev-parse", "refs/remotes/origin/_armature")
	runGit(t, worktree, "remote", "set-url", "origin", "file:///no/such/origin-for-d11-fail.git")

	d11 := checkD11OpsRemote(worktree, "", time.Now())
	assert.Equal(t, SeverityError, d11.Severity)
	assert.Contains(t, d11.Message, "Could not verify")
	assert.Equal(t, 1, d11DoctorExit(d11, false))
	after := gitOut(t, worktree, "rev-parse", "refs/remotes/origin/_armature")
	assert.Equal(t, before, after, "failed D11 must not rewrite shared tracking")
}

func TestLoadLocalOnlyOpsStaleAfter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, d11StaleLoad{after: defaultLocalOnlyOpsStaleAfter}, loadLocalOnlyOpsStaleAfter(""))
	missing := loadLocalOnlyOpsStaleAfter(filepath.Join(t.TempDir(), "missing.json"))
	assert.Equal(t, defaultLocalOnlyOpsStaleAfter, missing.after)
	assert.Empty(t, missing.fallbackWarn, "missing config fails open without a fallback warning")
	assert.Empty(t, missing.convertErr)

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"local_only_ops_stale_hours":6}`), 0o644))
	got := loadLocalOnlyOpsStaleAfter(path)
	assert.Equal(t, 6*time.Hour, got.after)
	assert.Empty(t, got.fallbackWarn)
	assert.Empty(t, got.convertErr)

	bad := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{not-json`), 0o644))
	fallback := loadLocalOnlyOpsStaleAfter(bad)
	assert.Equal(t, defaultLocalOnlyOpsStaleAfter, fallback.after)
	assert.Contains(t, fallback.fallbackWarn, "using default 24h")
	assert.NotEmpty(t, fallback.fallbackWarn)
	assert.Empty(t, fallback.convertErr)

	// Present 0: D10 rejects; D11 must use the same ValidatePresentFields path
	// and must not omit→24h.
	zeroPath := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(zeroPath, []byte(`{"local_only_ops_stale_hours":0}`), 0o644))
	zero := loadLocalOnlyOpsStaleAfter(zeroPath)
	assert.Zero(t, zero.after)
	assert.Empty(t, zero.fallbackWarn)
	assert.Contains(t, zero.convertErr, "local_only_ops_stale_hours")
	assert.Contains(t, zero.convertErr, "must be > 0")
	d11Zero := checkD11OpsRemote(t.TempDir(), zeroPath, time.Now())
	assert.Equal(t, SeverityError, d11Zero.Severity)
	assert.Contains(t, d11Zero.Message, "Invalid local_only_ops_stale_hours")

	// Overflow must not silently fall back to 24h (review example 2562048).
	overflowHours := int(math.MaxInt64/int64(time.Hour)) + 1
	overflowPath := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(overflowPath, []byte(fmt.Sprintf(`{"local_only_ops_stale_hours":%d}`, overflowHours)), 0o644))
	overflow := loadLocalOnlyOpsStaleAfter(overflowPath)
	assert.Zero(t, overflow.after, "overflow must not produce a usable threshold")
	assert.Empty(t, overflow.fallbackWarn, "overflow is not a silent 24h fallback")
	assert.NotEmpty(t, overflow.convertErr)

	d11 := checkD11OpsRemote(t.TempDir(), overflowPath, time.Now())
	assert.Equal(t, SeverityError, d11.Severity)
	assert.Contains(t, d11.Message, "Invalid local_only_ops_stale_hours")
	assert.Equal(t, 1, d11DoctorExit(d11, false))
}

func runGitEnv(t *testing.T, dir string, env map[string]string, args ...string) {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	cmd.Env = append(os.Environ(), cmd.Env...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func listRefs(t *testing.T, dir, prefix string) []string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, "for-each-ref", "--format=%(refname)", prefix)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "for-each-ref: %s", out)
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
