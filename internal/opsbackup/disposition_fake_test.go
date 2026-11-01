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
	fetchBranchErr      error
	originTip           string // resolved after successful FetchBranchToRef
	originResolveErr    error
	recoveredTip        string
	recoveredResolveErr error
	ancestor            map[[2]string]bool // (anc,desc) → is-ancestor
	ancestorErr         error
	bundleCreateErr     error
	bundleVerifyErr     error
	fetchBundleErr      error
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
	case strings.HasPrefix(rev, "refs/remotes/backup/_armature-run/"):
		if g.recoveredResolveErr != nil {
			return "", g.recoveredResolveErr
		}
		return g.recoveredTip, nil
	default:
		return "", fmt.Errorf("fakeGit: unexpected rev %q", rev)
	}
}

func (g *fakeGit) IsAncestor(anc, desc string) (bool, error) {
	if g.ancestorErr != nil {
		return false, g.ancestorErr
	}
	if g.ancestor == nil {
		return false, nil
	}
	return g.ancestor[[2]string{anc, desc}], nil
}

func (g *fakeGit) BundleCreateAdvertised(string, string, string) error {
	return g.bundleCreateErr
}

func (g *fakeGit) BundleVerifyRestorable(string, string) error {
	return g.bundleVerifyErr
}

func (g *fakeGit) FetchBundleForce(string, string, string) error {
	return g.fetchBundleErr
}

func (g *fakeGit) DeleteRef(ref string) error {
	g.deleted = append(g.deleted, ref)
	return nil
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
	missingFetch := errors.New("fatal: couldn't find remote ref refs/heads/_armature")

	cases := []struct {
		name           string
		g              *fakeGit
		bundle         string
		wantDisp       opsbackup.Disposition
		wantAside      string
		wantErrIs      error
		wantErrContain string
		// wantMissingSuccess is the PRRT anti-class: must NOT succeed as missing.
		forbidMissingSuccess bool
	}{
		{
			name:     "missing_fetch_reports_absent",
			g:        &fakeGit{fetchBranchErr: missingFetch, recoveredTip: recoveredSHA},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionMissing,
		},
		{
			name:     "fast_forward_equal_tips",
			g:        &fakeGit{originTip: originSHA, recoveredTip: originSHA},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionFastForward,
		},
		{
			name: "fast_forward_origin_ancestor_of_recovered",
			g: &fakeGit{
				originTip:    originSHA,
				recoveredTip: recoveredSHA,
				ancestor:     map[[2]string]bool{{originSHA, recoveredSHA}: true},
			},
			bundle:   "/tmp/a.bundle",
			wantDisp: opsbackup.DispositionFastForward,
		},
		{
			name: "diverge_unrelated",
			g: &fakeGit{
				originTip:    originSHA,
				recoveredTip: recoveredSHA,
				ancestor:     map[[2]string]bool{},
			},
			bundle:    "/tmp/a.bundle",
			wantDisp:  opsbackup.DispositionDivergeAside,
			wantAside: opsbackup.AsideRef,
			wantErrIs: opsbackup.ErrDiverge,
		},
		{
			// Ancestor decision: older recovered tip strictly ancestor of
			// current origin → diverge (I2: no shared-tip rewind).
			name: "diverge_recovered_strict_ancestor_of_origin",
			g: &fakeGit{
				originTip:    originSHA,
				recoveredTip: olderSHA,
				// origin is NOT ancestor of older; older IS ancestor of origin
				ancestor: map[[2]string]bool{{olderSHA, originSHA}: true},
			},
			bundle:    "/tmp/a.bundle",
			wantDisp:  opsbackup.DispositionDivergeAside,
			wantAside: opsbackup.AsideRef,
			wantErrIs: opsbackup.ErrDiverge,
		},
		{
			name:                 "fetch_fails_not_missing",
			g:                    &fakeGit{fetchBranchErr: errors.New("Could not resolve host"), recoveredTip: recoveredSHA},
			bundle:               "/tmp/a.bundle",
			wantErrContain:       "fail-closed",
			forbidMissingSuccess: true,
		},
		{
			// Codex PRRT_kwDORnVQE86ybIUQ: fetch ok, resolve fails → error, not missing.
			name: "fetch_ok_origin_resolve_fails_not_missing",
			g: &fakeGit{
				originResolveErr: errors.New("resolve origin tip boom"),
				recoveredTip:     recoveredSHA,
			},
			bundle:               "/tmp/a.bundle",
			wantErrContain:       "resolve origin tip",
			forbidMissingSuccess: true,
		},
		{
			name: "fetch_ok_origin_resolve_empty_not_missing",
			g: &fakeGit{
				originTip:    "",
				recoveredTip: recoveredSHA,
			},
			bundle:               "/tmp/a.bundle",
			wantErrContain:       "empty SHA",
			forbidMissingSuccess: true,
		},
		{
			name:           "bundle_verify_fails",
			g:              &fakeGit{bundleVerifyErr: errors.New("bad pack"), originTip: originSHA, recoveredTip: recoveredSHA},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "verify",
		},
		{
			name:           "fetch_bundle_fails",
			g:              &fakeGit{originTip: originSHA, fetchBundleErr: errors.New("load boom")},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "load bundle",
		},
		{
			name: "recovered_resolve_fails",
			g: &fakeGit{
				originTip:           originSHA,
				recoveredResolveErr: errors.New("scratch boom"),
			},
			bundle:         "/tmp/a.bundle",
			wantErrContain: "resolve recovered tip",
		},
		{
			name: "ancestry_check_fails",
			g: &fakeGit{
				originTip:    originSHA,
				recoveredTip: recoveredSHA,
				ancestorErr:  errors.New("ancestry boom"),
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
					// DispositionMissing is iota 0; only a non-empty recovered
					// tip distinguishes a successful missing plan from a
					// zero-value error return.
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
