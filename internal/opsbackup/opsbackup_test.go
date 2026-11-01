package opsbackup_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/opsbackup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type refreshFailGit struct {
	inner *adapters.Client
	err   error
}

func (g refreshFailGit) FetchBranchToRef(string, string) error {
	return g.err
}

func (g refreshFailGit) ResolveRevision(rev string) (string, error) {
	return g.inner.ResolveRevision(rev)
}

func (g refreshFailGit) IsAncestor(anc, desc string) (bool, error) {
	return g.inner.IsAncestor(anc, desc)
}

func (g refreshFailGit) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	return g.inner.BundleCreateAdvertised(outputPath, tipSHA, advertiseRef)
}

func (g refreshFailGit) BundleVerifyRestorable(bundlePath, wantRef string) error {
	return g.inner.BundleVerifyRestorable(bundlePath, wantRef)
}

func (g refreshFailGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
}

func (g refreshFailGit) DeleteRef(ref string) error {
	return g.inner.DeleteRef(ref)
}

type bundleFailGit struct {
	inner *adapters.Client
	err   error
}

func (g bundleFailGit) FetchBranchToRef(branch, dstRef string) error {
	return g.inner.FetchBranchToRef(branch, dstRef)
}

func (g bundleFailGit) ResolveRevision(rev string) (string, error) {
	return g.inner.ResolveRevision(rev)
}

func (g bundleFailGit) IsAncestor(anc, desc string) (bool, error) {
	return g.inner.IsAncestor(anc, desc)
}

func (g bundleFailGit) BundleCreateAdvertised(string, string, string) error {
	return g.err
}

func (g bundleFailGit) BundleVerifyRestorable(bundlePath, wantRef string) error {
	return g.inner.BundleVerifyRestorable(bundlePath, wantRef)
}

func (g bundleFailGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
}

func (g bundleFailGit) DeleteRef(ref string) error {
	return g.inner.DeleteRef(ref)
}

type resolveFailGit struct {
	inner     *adapters.Client
	failRev   string
	err       error
	fetchErr  error
	bundleErr error
	ancErr    error
}

func (g resolveFailGit) FetchBranchToRef(branch, dstRef string) error {
	if g.fetchErr != nil {
		return g.fetchErr
	}
	return g.inner.FetchBranchToRef(branch, dstRef)
}

func (g resolveFailGit) ResolveRevision(rev string) (string, error) {
	if g.failRev != "" && (rev == g.failRev || strings.HasPrefix(rev, g.failRev+"/")) {
		return "", g.err
	}
	return g.inner.ResolveRevision(rev)
}

func (g resolveFailGit) IsAncestor(anc, desc string) (bool, error) {
	if g.ancErr != nil {
		return false, g.ancErr
	}
	return g.inner.IsAncestor(anc, desc)
}

func (g resolveFailGit) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	if g.bundleErr != nil {
		return g.bundleErr
	}
	return g.inner.BundleCreateAdvertised(outputPath, tipSHA, advertiseRef)
}

func (g resolveFailGit) BundleVerifyRestorable(bundlePath, wantRef string) error {
	return g.inner.BundleVerifyRestorable(bundlePath, wantRef)
}

func (g resolveFailGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
}

func (g resolveFailGit) DeleteRef(ref string) error {
	return g.inner.DeleteRef(ref)
}

type fetchBundleFailGit struct {
	inner *adapters.Client
	err   error
}

func (g fetchBundleFailGit) FetchBranchToRef(branch, dstRef string) error {
	return g.inner.FetchBranchToRef(branch, dstRef)
}

func (g fetchBundleFailGit) ResolveRevision(rev string) (string, error) {
	return g.inner.ResolveRevision(rev)
}

func (g fetchBundleFailGit) IsAncestor(anc, desc string) (bool, error) {
	return g.inner.IsAncestor(anc, desc)
}

func (g fetchBundleFailGit) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	return g.inner.BundleCreateAdvertised(outputPath, tipSHA, advertiseRef)
}

func (g fetchBundleFailGit) BundleVerifyRestorable(bundlePath, wantRef string) error {
	return g.inner.BundleVerifyRestorable(bundlePath, wantRef)
}

func (g fetchBundleFailGit) FetchBundleForce(string, string, string) error {
	return g.err
}

func (g fetchBundleFailGit) DeleteRef(ref string) error {
	return g.inner.DeleteRef(ref)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func setupOpsOrigin(t *testing.T) (repo string, origin string, tip string) {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	repo = fx.Dir
	origin = fx.Origin
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "-b", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	tip = gitOut(t, origin, "rev-parse", "refs/heads/_armature")
	return repo, origin, tip
}

func TestSnapshot_RefreshFail_DoesNotWriteBundle_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	out := filepath.Join(t.TempDir(), "should-not-exist.bundle")
	g := refreshFailGit{inner: c, err: fmt.Errorf("simulated refresh failure")}

	_, err := opsbackup.Snapshot(g, opsbackup.SnapshotRequest{OutputPath: out})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-closed")
	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr), "bundle must not be written after refresh failure")
}

func TestSnapshot_BundleFail_PreservesPreexistingOutput_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	out := filepath.Join(t.TempDir(), "preexisting.bundle")
	require.NoError(t, os.WriteFile(out, []byte("keep-me"), 0o600))

	g := bundleFailGit{inner: c, err: fmt.Errorf("simulated bundle create failure")}
	_, err := opsbackup.Snapshot(g, opsbackup.SnapshotRequest{OutputPath: out})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundle")

	got, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("keep-me"), got, "pre-existing --output must not be removed on snapshot failure")
}

func TestSnapshot_DoesNotMoveLiveArmature_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)

	// Divergent local tip so a buggy rewind would be visible.
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "local unpublished")
	beforeLive := gitOut(t, repo, "rev-parse", "refs/heads/_armature")
	beforeTracking := gitOut(t, repo, "rev-parse", "refs/remotes/origin/_armature")
	require.NotEqual(t, beforeLive, beforeTracking)

	out := filepath.Join(t.TempDir(), "armature-ops.bundle")
	res, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: out})
	require.NoError(t, err)
	assert.Equal(t, beforeTracking, res.Tip)

	afterLive := gitOut(t, repo, "rev-parse", "refs/heads/_armature")
	assert.Equal(t, beforeLive, afterLive, "live refs/heads/_armature must be untouched")
	assert.Equal(t, beforeTracking, gitOut(t, repo, "rev-parse", "refs/remotes/origin/_armature"),
		"shared tracking tip must not be rewritten by snapshot")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/origin/_armature-run"), "per-run tracking cleaned up")
}

func TestSnapshot_BundleAdvertisesRestorableTip_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	out := filepath.Join(t.TempDir(), "armature-ops.bundle")

	res, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: out})
	require.NoError(t, err)
	assert.Equal(t, tip, res.Tip)

	require.NoError(t, opsbackup.Verify(c, out))
	heads, err := c.BundleListHeads(out)
	require.NoError(t, err)
	assert.Contains(t, heads, opsbackup.AdvertisedRef)
}

func TestRestorePlan_DivergeAside_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, origin, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Replace origin/_armature with an unrelated orphan tip (non-FF rewrite).
	gittest.Git(t, repo, "checkout", "--orphan", "unrelated-ops")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "unrelated tip")
	unrelated := gitOut(t, repo, "rev-parse", "HEAD")
	gittest.Git(t, repo, "push", "--force", "origin", "HEAD:refs/heads/_armature")
	gittest.Git(t, repo, "checkout", "_armature")

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.ErrorIs(t, err, opsbackup.ErrDiverge)
	assert.Equal(t, opsbackup.DispositionDivergeAside, plan.Disposition)
	assert.Equal(t, opsbackup.AsideRef, plan.AsideRef)
	assert.Equal(t, bundle, plan.Bundle)
	assert.Equal(t, "diverge→_armature-recovered", plan.Disposition.String())

	originTip := gitOut(t, origin, "rev-parse", "refs/heads/_armature")
	assert.Equal(t, unrelated, originTip, "shared origin tip must remain the remote tip (aside only)")
}

func TestRestorePlan_AbsentRemoteRef_RealAdapter_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, origin, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Delete the shared branch on origin. Stale local tracking tip must not
	// become a false fast-forward; refresh reports absent → missing.
	gittest.Git(t, origin, "update-ref", "-d", "refs/heads/_armature")

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.NoError(t, err)
	assert.Equal(t, opsbackup.DispositionMissing, plan.Disposition)
	assert.Equal(t, "missing", plan.Disposition.String())
	assert.NotEmpty(t, plan.Recovered)
	assert.Equal(t, bundle, plan.Bundle)
}

func TestRestorePlan_UnreachableOrigin_FailClosed_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	gittest.Git(t, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-origin.git"))

	_, err = opsbackup.PlanRestore(c, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-closed")
	assert.False(t, adapters.IsRemoteRefMissing(err), "unreachable origin must not classify as missing")
}

func TestVerify_TruncatedBundle_RealAdapter_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	// Enlarge the tip so the pack is not tiny; header-only truncation must fail
	// when verified via fetch into an empty bare (not list-heads alone).
	blob := filepath.Join(repo, "payload.bin")
	require.NoError(t, os.WriteFile(blob, make([]byte, 64*1024), 0o600))
	gittest.Git(t, repo, "add", "payload.bin")
	gittest.Git(t, repo, "commit", "-m", "large tip")
	gittest.Git(t, repo, "push", "origin", "_armature")

	c := adapters.New(repo)
	full := filepath.Join(t.TempDir(), "full.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: full})
	require.NoError(t, err)
	require.NoError(t, opsbackup.Verify(c, full))

	raw, err := os.ReadFile(full)
	require.NoError(t, err)
	headerEnd := strings.Index(string(raw), "\n\n")
	require.Greater(t, headerEnd, 0)
	trunc := filepath.Join(t.TempDir(), "truncated.bundle")
	require.NoError(t, adapters.WriteFile(trunc, raw[:headerEnd+2], 0o600))

	heads, listErr := c.BundleListHeads(trunc)
	require.NoError(t, listErr, "list-heads alone is the false-green class under test")
	assert.Contains(t, heads, opsbackup.AdvertisedRef)

	err = opsbackup.Verify(c, trunc)
	require.Error(t, err, "truncated pack must fail restorable verify")
	assert.Contains(t, err.Error(), "verify")
}

func TestRestorePlan_FastForward_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.NoError(t, err)
	assert.Equal(t, opsbackup.DispositionFastForward, plan.Disposition)
	assert.Equal(t, tip, plan.Recovered)
	assert.Equal(t, bundle, plan.Bundle)
	assert.Equal(t, "fast-forward", plan.Disposition.String())
}

func TestRestorePlan_FastForwardAncestor_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "newer.bundle")

	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "newer ops tip")
	gittest.Git(t, repo, "push", "origin", "_armature")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Rewind origin to an ancestor of the recovered tip (still FF-able).
	older := gitOut(t, repo, "rev-parse", "HEAD~1")
	gittest.Git(t, repo, "push", "--force", "origin", older+":refs/heads/_armature")

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.NoError(t, err)
	assert.Equal(t, opsbackup.DispositionFastForward, plan.Disposition)
}

// TestRestorePlan_RecoveredStrictAncestorOfOrigin_Diverge_REQ_TOPTIER_S12_T1
// locks the ancestor decision: an older bundle tip that is a strict ancestor
// of current origin/_armature is diverge→_armature-recovered (exit via
// ErrDiverge). I2 forbids rewinding the shared tip; publish aside only.
func TestRestorePlan_RecoveredStrictAncestorOfOrigin_Diverge_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, origin, olderTip := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "older.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Advance origin past the bundled tip so recovered is a strict ancestor.
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "newer than bundle")
	gittest.Git(t, repo, "push", "origin", "_armature")
	newerTip := gitOut(t, origin, "rev-parse", "refs/heads/_armature")
	require.NotEqual(t, olderTip, newerTip)

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.ErrorIs(t, err, opsbackup.ErrDiverge)
	assert.Equal(t, opsbackup.DispositionDivergeAside, plan.Disposition)
	assert.Equal(t, opsbackup.AsideRef, plan.AsideRef)
	assert.Equal(t, olderTip, plan.Recovered)
	assert.Equal(t, newerTip, gitOut(t, origin, "rev-parse", "refs/heads/_armature"),
		"shared origin tip must remain advanced (aside only)")
}

func TestPlanRestore_FetchOkOriginResolveFails_NotMissing_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Codex PRRT_kwDORnVQE86ybIUQ: fetch succeeds, ResolveRevision on the
	// per-run origin tip fails → fail closed, never successful missing.
	g := resolveFailGit{inner: c, failRev: "refs/remotes/origin/_armature-run", err: fmt.Errorf("resolve origin tip boom")}
	plan, err := opsbackup.PlanRestore(g, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve origin tip")
	assert.Empty(t, plan.Recovered, "fail-closed must not emit a successful plan")
	assert.Empty(t, plan.Bundle)
	assert.NotContains(t, strings.ToLower(err.Error()), "missing")
}

func TestSnapshot_RequiresOutput(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	_, err := opsbackup.Snapshot(adapters.New(repo), opsbackup.SnapshotRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--output")
}

func TestSnapshot_ResolveTrackingFail(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	// Fail resolve of any per-run tracking destination under the sibling NS.
	g := resolveFailGit{inner: c, failRev: "refs/remotes/origin/_armature-run", err: fmt.Errorf("resolve boom")}
	_, err := opsbackup.Snapshot(g, opsbackup.SnapshotRequest{OutputPath: filepath.Join(t.TempDir(), "x.bundle")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
}

func TestVerify_RequiresPathAndAdvertisement(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	require.Error(t, opsbackup.Verify(c, ""))

	other := filepath.Join(t.TempDir(), "other.bundle")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "extra")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	require.NoError(t, c.BundleCreateAdvertised(other, sha, "refs/heads/not-armature"))
	err := opsbackup.Verify(c, other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not advertise")
}

func TestPlanRestore_RequiresFrom(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	_, err := opsbackup.PlanRestore(adapters.New(repo), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--from")
}

func TestDispositionString_Unknown(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "disposition(99)", opsbackup.Disposition(99).String())
}

func TestPlanRestore_RefreshFail_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	g := refreshFailGit{inner: c, err: fmt.Errorf("simulated restore refresh failure")}
	_, err = opsbackup.PlanRestore(g, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-closed")
}

func TestPlanRestore_FetchBundleFail(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	g := fetchBundleFailGit{inner: c, err: fmt.Errorf("simulated fetch bundle failure")}
	_, err = opsbackup.PlanRestore(g, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load bundle")
}

func TestPlanRestore_RecoveredResolveFail(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	g := resolveFailGit{inner: c, failRev: "refs/remotes/backup/_armature-run", err: fmt.Errorf("scratch boom")}
	_, err = opsbackup.PlanRestore(g, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve recovered tip")
}

func TestPlanRestore_AncestryFail(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "advance recovered")
	newerBundle := filepath.Join(t.TempDir(), "newer.bundle")
	require.NoError(t, c.BundleCreateAdvertised(newerBundle, gitOut(t, repo, "rev-parse", "HEAD"), opsbackup.AdvertisedRef))

	g := resolveFailGit{inner: c, ancErr: fmt.Errorf("ancestry boom")}
	_, err = opsbackup.PlanRestore(g, newerBundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ancestry")
}

// tipInjectGit lets concurrent Snapshot calls report distinct tips while using
// the real BundleCreateAdvertised path on one clone (staging-ref race class).
type tipInjectGit struct {
	inner *adapters.Client
	tips  []string
	n     atomic.Uint64
}

func (g *tipInjectGit) FetchBranchToRef(string, string) error { return nil }

func (g *tipInjectGit) ResolveRevision(rev string) (string, error) {
	if strings.HasPrefix(rev, "refs/remotes/origin/_armature-run/") {
		i := g.n.Add(1) - 1
		return g.tips[i%uint64(len(g.tips))], nil
	}
	return g.inner.ResolveRevision(rev)
}

func (g *tipInjectGit) IsAncestor(anc, desc string) (bool, error) {
	return g.inner.IsAncestor(anc, desc)
}

func (g *tipInjectGit) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	return g.inner.BundleCreateAdvertised(outputPath, tipSHA, advertiseRef)
}

func (g *tipInjectGit) BundleVerifyRestorable(bundlePath, wantRef string) error {
	return g.inner.BundleVerifyRestorable(bundlePath, wantRef)
}

func (g *tipInjectGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
}

func (g *tipInjectGit) DeleteRef(ref string) error {
	return g.inner.DeleteRef(ref)
}

func listRefs(t *testing.T, repo, pattern string) []string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(repo, "for-each-ref", "--format=%(refname)", pattern)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "for-each-ref %s: %s", pattern, out)
	var refs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			refs = append(refs, line)
		}
	}
	return refs
}

func bundleTip(t *testing.T, repo, bundle, wantRef string) string {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(repo, "bundle", "list-heads", bundle)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "bundle list-heads: %s", out)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[1] == wantRef {
			return fields[0]
		}
	}
	t.Fatalf("bundle %s does not advertise %s\n%s", bundle, wantRef, out)
	return ""
}

func TestSnapshot_ConcurrentDistinctTips_NoCrossWire_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, tipA := setupOpsOrigin(t)
	c := adapters.New(repo)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "tip B")
	tipB := gitOut(t, repo, "rev-parse", "HEAD")
	require.NotEqual(t, tipA, tipB)

	g := &tipInjectGit{inner: c, tips: []string{tipA, tipB}}
	dir := t.TempDir()
	outA := filepath.Join(dir, "a.bundle")
	outB := filepath.Join(dir, "b.bundle")

	var (
		wg   sync.WaitGroup
		resA opsbackup.SnapshotResult
		resB opsbackup.SnapshotResult
		errA error
		errB error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		resA, errA = opsbackup.Snapshot(g, opsbackup.SnapshotRequest{OutputPath: outA})
	}()
	go func() {
		defer wg.Done()
		resB, errB = opsbackup.Snapshot(g, opsbackup.SnapshotRequest{OutputPath: outB})
	}()
	wg.Wait()
	require.NoError(t, errA)
	require.NoError(t, errB)
	require.True(t, (resA.Tip == tipA && resB.Tip == tipB) || (resA.Tip == tipB && resB.Tip == tipA),
		"expected distinct tips %s/%s, got %s/%s", tipA, tipB, resA.Tip, resB.Tip)
	require.NotEqual(t, resA.Tip, resB.Tip)

	assert.Equal(t, resA.Tip, bundleTip(t, repo, outA, opsbackup.AdvertisedRef), "bundle A must match reported tip")
	assert.Equal(t, resB.Tip, bundleTip(t, repo, outB, opsbackup.AdvertisedRef), "bundle B must match reported tip")
	assert.Empty(t, listRefs(t, repo, "refs/bundle-backup"), "no stray bundle-backup staging refs")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/origin/_armature-run"), "per-run tracking cleaned up")
}

func TestVerify_ConcurrentDistinctBundles_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, tipA := setupOpsOrigin(t)
	c := adapters.New(repo)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "tip B")
	tipB := gitOut(t, repo, "rev-parse", "HEAD")

	dir := t.TempDir()
	bundleA := filepath.Join(dir, "a.bundle")
	bundleB := filepath.Join(dir, "b.bundle")
	require.NoError(t, c.BundleCreateAdvertised(bundleA, tipA, opsbackup.AdvertisedRef))
	require.NoError(t, c.BundleCreateAdvertised(bundleB, tipB, opsbackup.AdvertisedRef))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = opsbackup.Verify(c, bundleA)
	}()
	go func() {
		defer wg.Done()
		errs[1] = opsbackup.Verify(c, bundleB)
	}()
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
}

func TestRestorePlan_ConcurrentDistinctBundles_NoCrossWire_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, tipA := setupOpsOrigin(t)
	c := adapters.New(repo)
	// tipB is a local fast-forward of origin (still at tipA) so both plans succeed.
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "tip B")
	tipB := gitOut(t, repo, "rev-parse", "HEAD")
	require.NotEqual(t, tipA, tipB)

	dir := t.TempDir()
	bundleA := filepath.Join(dir, "a.bundle")
	bundleB := filepath.Join(dir, "b.bundle")
	require.NoError(t, c.BundleCreateAdvertised(bundleA, tipA, opsbackup.AdvertisedRef))
	require.NoError(t, c.BundleCreateAdvertised(bundleB, tipB, opsbackup.AdvertisedRef))

	var (
		wg    sync.WaitGroup
		planA opsbackup.RestorePlan
		planB opsbackup.RestorePlan
		errA  error
		errB  error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		planA, errA = opsbackup.PlanRestore(c, bundleA)
	}()
	go func() {
		defer wg.Done()
		planB, errB = opsbackup.PlanRestore(c, bundleB)
	}()
	wg.Wait()
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, tipA, planA.Recovered)
	assert.Equal(t, tipB, planB.Recovered)
	assert.Equal(t, opsbackup.DispositionFastForward, planA.Disposition)
	assert.Equal(t, opsbackup.DispositionFastForward, planB.Disposition)
	assert.Equal(t, bundleA, planA.Bundle)
	assert.Equal(t, bundleB, planB.Bundle)
	assert.Empty(t, listRefs(t, repo, "refs/remotes/backup/_armature-run"), "per-run scratch cleaned up")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/origin/_armature-run"), "per-run tracking cleaned up")
	assert.Empty(t, listRefs(t, repo, "refs/bundle-backup"), "no stray bundle-backup staging refs")
}

// TestRestorePlan_LegacyScratchParentStillWorks_REQ_TOPTIER_S12_T1F proves the
// Codex class: a leftover direct refs/remotes/backup/_armature must not block
// sibling-namespace per-run refs.
func TestRestorePlan_LegacyScratchParentStillWorks_REQ_TOPTIER_S12_T1F(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	// Pre-create the legacy fixed tip that nested children cannot coexist with.
	gittest.Git(t, repo, "update-ref", "refs/remotes/backup/_armature", tip)
	require.Equal(t, tip, gitOut(t, repo, "rev-parse", "refs/remotes/backup/_armature"))

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.NoError(t, err)
	assert.Equal(t, opsbackup.DispositionFastForward, plan.Disposition)
	assert.Equal(t, tip, plan.Recovered)
	assert.Equal(t, tip, gitOut(t, repo, "rev-parse", "refs/remotes/backup/_armature"),
		"legacy parent tip must remain untouched")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/backup/_armature-run"))
}

// TestOpsBackup_CrashLeftoversDoNotBreakNextRun_REQ_TOPTIER_S12_T1F leaves stale
// per-run refs and temp dirs (crash simulation); the next invocation succeeds.
func TestOpsBackup_CrashLeftoversDoNotBreakNextRun_REQ_TOPTIER_S12_T1F(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	staleTrack := "refs/remotes/origin/_armature-run/deadbeefcafebabe"
	staleScratch := "refs/remotes/backup/_armature-run/deadbeefcafebabe"
	gittest.Git(t, repo, "update-ref", staleTrack, tip)
	gittest.Git(t, repo, "update-ref", staleScratch, tip)
	staleBare := filepath.Join(os.TempDir(), "armature-ops-backup-bare-stale-t1f")
	staleVerify := filepath.Join(os.TempDir(), "armature-ops-backup-verify-stale-t1f")
	require.NoError(t, os.MkdirAll(filepath.Join(staleBare, "bare.git"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(staleVerify, "bare.git"), 0o750))
	t.Cleanup(func() {
		_ = os.RemoveAll(staleBare)   //nolint:errcheck // best-effort crash-sim leftover cleanup
		_ = os.RemoveAll(staleVerify) //nolint:errcheck // best-effort crash-sim leftover cleanup
	})

	out := filepath.Join(t.TempDir(), "after-crash.bundle")
	res, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: out})
	require.NoError(t, err)
	assert.Equal(t, tip, res.Tip)
	require.NoError(t, opsbackup.Verify(c, out))
	plan, err := opsbackup.PlanRestore(c, out)
	require.NoError(t, err)
	assert.Equal(t, tip, plan.Recovered)

	// Stale leftovers remain (we do not sweep foreign nonces) but do not block.
	assert.Equal(t, tip, gitOut(t, repo, "rev-parse", staleTrack))
	assert.Equal(t, tip, gitOut(t, repo, "rev-parse", staleScratch))
}

// TestRestorePlan_DurableOutputResolvesAfterExit_REQ_TOPTIER_S12_T1F: every
// printed path/SHA still resolves after PlanRestore returns (no scratch ref).
func TestRestorePlan_DurableOutputResolvesAfterExit_REQ_TOPTIER_S12_T1F(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	plan, err := opsbackup.PlanRestore(c, bundle)
	require.NoError(t, err)
	assert.Equal(t, tip, plan.Recovered)
	assert.Equal(t, bundle, plan.Bundle)
	assert.Empty(t, plan.AsideRef)

	_, err = os.Stat(plan.Bundle)
	require.NoError(t, err, "bundle path must exist after exit")
	require.Equal(t, tip, gitOut(t, repo, "rev-parse", "--verify", plan.Recovered+"^{commit}"),
		"recovered SHA must resolve after exit")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/backup/_armature-run"))
	assert.Empty(t, listRefs(t, repo, "refs/remotes/origin/_armature-run"))
}

func TestOpsBackup_ParallelAllVerbsOverlap_REQ_TOPTIER_S12_T1F(t *testing.T) {
	t.Parallel()
	repo, _, tip := setupOpsOrigin(t)
	c := adapters.New(repo)
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: seed})
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 6)
	wg.Add(6)
	go func() {
		defer wg.Done()
		_, errs[0] = opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: filepath.Join(dir, "s1.bundle")})
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: filepath.Join(dir, "s2.bundle")})
	}()
	go func() {
		defer wg.Done()
		errs[2] = opsbackup.Verify(c, seed)
	}()
	go func() {
		defer wg.Done()
		errs[3] = opsbackup.Verify(c, seed)
	}()
	go func() {
		defer wg.Done()
		var plan opsbackup.RestorePlan
		plan, errs[4] = opsbackup.PlanRestore(c, seed)
		if errs[4] == nil && plan.Recovered != tip {
			errs[4] = fmt.Errorf("unexpected recovered tip %s", plan.Recovered)
		}
	}()
	go func() {
		defer wg.Done()
		var plan opsbackup.RestorePlan
		plan, errs[5] = opsbackup.PlanRestore(c, seed)
		if errs[5] == nil && plan.Recovered != tip {
			errs[5] = fmt.Errorf("unexpected recovered tip %s", plan.Recovered)
		}
	}()
	wg.Wait()
	for i, e := range errs {
		require.NoErrorf(t, e, "parallel verb slot %d", i)
	}
}

func TestOpsBackup_ParallelMovingTip_NoSharedTrackingLock_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, origin, tip0 := setupOpsOrigin(t)
	c := adapters.New(repo)

	// Prove the shared tracking tip stays absent: delete the push -u tip and
	// require FetchBranchToRef (--refmap=) never recreates it under load.
	gittest.Git(t, repo, "update-ref", "-d", "refs/remotes/origin/_armature")

	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.bundle")
	seedRes, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: seed})
	require.NoError(t, err)
	require.Equal(t, tip0, seedRes.Tip)

	gitQuiet := func(cwd string, args ...string) (string, error) {
		out, cmdErr := adapters.NonInteractiveGitCommand(cwd, args...).CombinedOutput()
		if cmdErr != nil {
			return "", fmt.Errorf("git %v: %w\n%s", args, cmdErr, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	bundleTipQuiet := func(bundle string) (string, error) {
		out, cmdErr := adapters.NonInteractiveGitCommand(repo, "bundle", "list-heads", bundle).CombinedOutput()
		if cmdErr != nil {
			return "", fmt.Errorf("bundle list-heads: %w\n%s", cmdErr, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) >= 2 && fields[1] == opsbackup.AdvertisedRef {
				return fields[0], nil
			}
		}
		return "", fmt.Errorf("bundle %s does not advertise %s", bundle, opsbackup.AdvertisedRef)
	}

	stop := make(chan struct{})
	var moverErr error
	var moverWg sync.WaitGroup
	moverWg.Add(1)
	go func() {
		defer moverWg.Done()
		parent := tip0
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			tree, treeErr := gitQuiet(origin, "rev-parse", parent+"^{tree}")
			if treeErr != nil {
				moverErr = treeErr
				return
			}
			next, commitErr := gitQuiet(origin, "commit-tree", tree, "-p", parent, "-m", fmt.Sprintf("moving tip %d", i))
			if commitErr != nil {
				moverErr = commitErr
				return
			}
			if _, updErr := gitQuiet(origin, "update-ref", "refs/heads/_armature", next); updErr != nil {
				moverErr = updErr
				return
			}
			parent = next
		}
	}()

	const workers = 12
	var wg sync.WaitGroup
	errs := make([]error, workers)
	reportedTips := make([]string, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				out := filepath.Join(dir, fmt.Sprintf("snap-%d.bundle", i))
				res, snapErr := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: out})
				if snapErr != nil {
					errs[i] = snapErr
					return
				}
				reportedTips[i] = res.Tip
				got, tipErr := bundleTipQuiet(out)
				if tipErr != nil {
					errs[i] = tipErr
					return
				}
				if got != res.Tip {
					errs[i] = fmt.Errorf("bundle tip %s != reported %s", got, res.Tip)
				}
			case 1:
				if vErr := opsbackup.Verify(c, seed); vErr != nil {
					errs[i] = vErr
				}
			default:
				plan, pErr := opsbackup.PlanRestore(c, seed)
				if pErr != nil && !errors.Is(pErr, opsbackup.ErrDiverge) {
					errs[i] = pErr
					return
				}
				if plan.Recovered == "" {
					errs[i] = fmt.Errorf("restore-plan returned empty recovered tip")
				}
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	moverWg.Wait()
	require.NoError(t, moverErr, "origin tip mover failed")

	for i, e := range errs {
		if e != nil {
			require.NotContains(t, e.Error(), "cannot lock ref", "slot %d: %v", i, e)
		}
		require.NoErrorf(t, e, "parallel moving-tip verb slot %d", i)
	}
	finalTip := gitOut(t, origin, "rev-parse", "refs/heads/_armature")
	oc := adapters.New(origin)
	for i, tip := range reportedTips {
		if tip == "" {
			continue
		}
		if tip == finalTip || tip == tip0 {
			continue
		}
		anc, ancErr := oc.IsAncestor(tip, finalTip)
		require.NoError(t, ancErr)
		require.Truef(t, anc, "snapshot slot %d tip %s must be on the moving origin/_armature history (final=%s)", i, tip, finalTip)
	}

	_, trackErr := adapters.NonInteractiveGitCommand(repo, "rev-parse", "refs/remotes/origin/_armature").CombinedOutput()
	require.Error(t, trackErr, "refs/remotes/origin/_armature must stay absent before/after")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/origin/_armature-run"), "per-run tracking cleaned up")
	assert.Empty(t, listRefs(t, repo, "refs/remotes/backup/_armature-run"), "per-run scratch cleaned up")
}
