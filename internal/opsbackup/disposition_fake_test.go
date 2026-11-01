package opsbackup_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/opsbackup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGit is a pure in-memory Git port for disposition / fail-closed tables.
// Call sites inject typed outcomes; no real git process is required.
type fakeGit struct {
	fetchBranchErr   error
	originTip        string // resolved after successful FetchBranchToRef
	originResolveErr error
	bundleCreateErr  error
	bundleVerifyErr  error

	restoreMissing      bool
	restoreOriginTip    string
	restoreRecovered    string
	restoreOriginAnc    bool
	restoreRecoveredAnc bool
	restoreEvidenceErr  error
	deleted             []string
}

func (g *fakeGit) FetchBranchToRef(string, string) error {
	return g.fetchBranchErr
}

func (g *fakeGit) ResolveRevision(rev string) (string, error) {
	switch {
	case strings.HasPrefix(rev, "refs/remotes/origin/_armature-run/"):
		if g.originResolveErr != nil {
			return "", g.originResolveErr
		}
		return g.originTip, nil
	default:
		return "", fmt.Errorf("fakeGit: unexpected rev %q", rev)
	}
}

func (g *fakeGit) BundleCreateAdvertised(string, string, string) error {
	return g.bundleCreateErr
}

func (g *fakeGit) BundleVerifyRestorable(string, string) error {
	return g.bundleVerifyErr
}

func (g *fakeGit) DeleteRef(ref string) error {
	g.deleted = append(g.deleted, ref)
	return nil
}

func (g *fakeGit) RestoreEvidence(string, string, string) (bool, string, string, bool, bool, error) {
	if g.restoreEvidenceErr != nil {
		return false, "", "", false, false, g.restoreEvidenceErr
	}
	return g.restoreMissing, g.restoreOriginTip, g.restoreRecovered, g.restoreOriginAnc, g.restoreRecoveredAnc, nil
}

func TestSnapshot_FakeGitTable_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	const tip = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := []struct {
		name       string
		g          *fakeGit
		output     string
		wantErr    string
		wantTip    string
		wantBundle bool
	}{
		{
			name:       "ok",
			g:          &fakeGit{originTip: tip},
			output:     "out.bundle",
			wantTip:    tip,
			wantBundle: true,
		},
		{
			name:    "empty_output",
			g:       &fakeGit{originTip: tip},
			output:  "",
			wantErr: "--output",
		},
		{
			name:    "fetch_fails",
			g:       &fakeGit{fetchBranchErr: errors.New("network down")},
			output:  "out.bundle",
			wantErr: "fail-closed",
		},
		{
			name:    "fetch_ok_resolve_fails",
			g:       &fakeGit{originResolveErr: errors.New("resolve boom")},
			output:  "out.bundle",
			wantErr: "resolve",
		},
		{
			name:    "bundle_create_fails",
			g:       &fakeGit{originTip: tip, bundleCreateErr: errors.New("bundle boom")},
			output:  "out.bundle",
			wantErr: "bundle",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := ""
			if tc.output != "" {
				out = t.TempDir() + "/" + tc.output
			}
			res, err := opsbackup.Snapshot(tc.g, opsbackup.SnapshotRequest{OutputPath: out})
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantTip, res.Tip)
			if tc.wantBundle {
				assert.Equal(t, out, res.Bundle)
			}
		})
	}
}

func TestVerify_FakeGitTable_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		g       *fakeGit
		bundle  string
		wantErr string
	}{
		{
			name:   "ok",
			g:      &fakeGit{},
			bundle: "/tmp/ok.bundle",
		},
		{
			name:    "empty_path",
			g:       &fakeGit{},
			bundle:  "",
			wantErr: "bundle path",
		},
		{
			name:    "verify_fails",
			g:       &fakeGit{bundleVerifyErr: errors.New("truncated pack")},
			bundle:  "/tmp/bad.bundle",
			wantErr: "verify",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := opsbackup.Verify(tc.g, tc.bundle)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPlanRestore_FakeGitDispositionTable_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	const (
		originSHA    = "1111111111111111111111111111111111111111"
		recoveredSHA = "2222222222222222222222222222222222222222"
		olderSHA     = "0000000000000000000000000000000000000001"
	)

	cases := []struct {
		name                 string
		g                    *fakeGit
		bundle               string
		wantDisp             opsbackup.Disposition
		wantAside            string
		wantErrIs            error
		wantErrContain       string
		forbidMissingSuccess bool
	}{
		{
			name: "missing_ls_remote_reports_absent",
			g: &fakeGit{
				restoreMissing:   true,
				restoreRecovered: recoveredSHA,
			},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionMissing,
		},
		{
			name: "fast_forward_equal_tips",
			g: &fakeGit{
				restoreOriginTip:    originSHA,
				restoreRecovered:    originSHA,
				restoreOriginAnc:    true,
				restoreRecoveredAnc: true,
			},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionFastForward,
		},
		{
			name: "fast_forward_origin_ancestor_of_recovered",
			g: &fakeGit{
				restoreOriginTip: originSHA,
				restoreRecovered: recoveredSHA,
				restoreOriginAnc: true,
			},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionFastForward,
		},
		{
			name: "diverge_unrelated",
			g: &fakeGit{
				restoreOriginTip: originSHA,
				restoreRecovered: recoveredSHA,
			},
			bundle:    "/tmp/a.bundle",
			wantDisp:  opsbackup.DispositionDivergeAside,
			wantAside: opsbackup.AsideRef,
			wantErrIs: opsbackup.ErrDiverge,
		},
		{
			// Ancestor decision: older recovered tip strictly ancestor of
			// current origin → already-contained (exit 0, no aside).
			name: "already_contained_recovered_strict_ancestor_of_origin",
			g: &fakeGit{
				restoreOriginTip:    originSHA,
				restoreRecovered:    olderSHA,
				restoreRecoveredAnc: true,
			},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionAlreadyContained,
		},
		{
			name: "ls_remote_unreachable_fail_closed_not_missing",
			g: &fakeGit{
				restoreEvidenceErr: errors.New("ls-remote origin refs/heads/_armature failed (fail-closed): Could not resolve host"),
			},
			bundle:               "/tmp/a.bundle",
			wantErrContain:       "fail-closed",
			forbidMissingSuccess: true,
		},
		{
			name: "origin_fetch_ok_resolve_fails_not_missing",
			g: &fakeGit{
				restoreEvidenceErr: errors.New("resolve origin tip boom"),
			},
			bundle:               "/tmp/a.bundle",
			wantErrContain:       "resolve origin tip",
			forbidMissingSuccess: true,
		},
		{
			name:           "bundle_verify_fails",
			g:              &fakeGit{bundleVerifyErr: errors.New("bad pack")},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "verify",
		},
		{
			name: "restore_evidence_load_bundle_fails",
			g: &fakeGit{
				restoreEvidenceErr: errors.New("load bundle boom"),
			},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "load bundle",
		},
		{
			name: "ancestry_check_fails",
			g: &fakeGit{
				restoreEvidenceErr: errors.New("ancestry boom"),
			},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "ancestry",
		},
		{
			name:           "empty_from",
			g:              &fakeGit{},
			bundle:         "",
			wantErrContain: "--from",
		},
		{
			name: "empty_recovered_fail_closed",
			g: &fakeGit{
				restoreMissing:   true,
				restoreRecovered: "",
			},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "empty SHA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := opsbackup.PlanRestore(tc.g, tc.bundle)
			if tc.wantErrContain != "" || tc.wantErrIs != nil {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					require.ErrorIs(t, err, tc.wantErrIs)
					assert.Equal(t, tc.wantDisp, plan.Disposition)
					assert.Equal(t, tc.wantAside, plan.AsideRef)
				}
				if tc.wantErrContain != "" {
					assert.Contains(t, err.Error(), tc.wantErrContain)
				}
				if tc.forbidMissingSuccess {
					require.Error(t, err)
					assert.Empty(t, plan.Recovered, "fail-closed path must not return a recovered tip plan")
					assert.Empty(t, plan.Bundle)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDisp, plan.Disposition)
			assert.Equal(t, tc.wantAside, plan.AsideRef)
			assert.Equal(t, tc.bundle, plan.Bundle)
			assert.NotEmpty(t, plan.Recovered)
		})
	}
}
