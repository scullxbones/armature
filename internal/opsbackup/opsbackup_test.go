package opsbackup_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func (g refreshFailGit) FetchTrackingRefWithoutMovingHEAD(string) error {
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

type bundleFailGit struct {
	inner *adapters.Client
	err   error
}

func (g bundleFailGit) FetchTrackingRefWithoutMovingHEAD(branch string) error {
	return g.inner.FetchTrackingRefWithoutMovingHEAD(branch)
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

type resolveFailGit struct {
	inner     *adapters.Client
	failRev   string
	err       error
	fetchErr  error
	bundleErr error
	ancErr    error
}

func (g resolveFailGit) FetchTrackingRefWithoutMovingHEAD(branch string) error {
	if g.fetchErr != nil {
		return g.fetchErr
	}
	return g.inner.FetchTrackingRefWithoutMovingHEAD(branch)
}

func (g resolveFailGit) ResolveRevision(rev string) (string, error) {
	if g.failRev != "" && rev == g.failRev {
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

type fetchBundleFailGit struct {
	inner *adapters.Client
	err   error
}

func (g fetchBundleFailGit) FetchTrackingRefWithoutMovingHEAD(branch string) error {
	return g.inner.FetchTrackingRefWithoutMovingHEAD(branch)
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
	g := resolveFailGit{inner: c, failRev: opsbackup.TrackingRef, err: fmt.Errorf("resolve boom")}
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

func TestPlanRestore_ScratchResolveFail(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	g := resolveFailGit{inner: c, failRev: opsbackup.ScratchRef, err: fmt.Errorf("scratch boom")}
	_, err = opsbackup.PlanRestore(g, bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve scratch")
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
