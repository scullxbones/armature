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

func (g refreshFailGit) BundleListHeads(bundlePath string) ([]string, error) {
	return g.inner.BundleListHeads(bundlePath)
}

func (g refreshFailGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
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

func TestRestorePlan_Missing_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	c := adapters.New(repo)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")
	_, err := opsbackup.Snapshot(c, opsbackup.SnapshotRequest{OutputPath: bundle})
	require.NoError(t, err)

	g := missingAfterRefreshGit{inner: c}
	plan, err := opsbackup.PlanRestore(g, bundle)
	require.NoError(t, err)
	assert.Equal(t, opsbackup.DispositionMissing, plan.Disposition)
	assert.Equal(t, "missing", plan.Disposition.String())
}

type missingAfterRefreshGit struct {
	inner *adapters.Client
}

func (g missingAfterRefreshGit) FetchTrackingRefWithoutMovingHEAD(string) error {
	// Pretend refresh succeeded but left no tracking tip (deleted upstream).
	return g.inner.DeleteRef(opsbackup.TrackingRef)
}

func (g missingAfterRefreshGit) ResolveRevision(rev string) (string, error) {
	return g.inner.ResolveRevision(rev)
}

func (g missingAfterRefreshGit) IsAncestor(anc, desc string) (bool, error) {
	return g.inner.IsAncestor(anc, desc)
}

func (g missingAfterRefreshGit) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	return g.inner.BundleCreateAdvertised(outputPath, tipSHA, advertiseRef)
}

func (g missingAfterRefreshGit) BundleListHeads(bundlePath string) ([]string, error) {
	return g.inner.BundleListHeads(bundlePath)
}

func (g missingAfterRefreshGit) FetchBundleForce(bundlePath, srcRef, dstRef string) error {
	return g.inner.FetchBundleForce(bundlePath, srcRef, dstRef)
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

func TestSnapshot_RequiresOutput(t *testing.T) {
	t.Parallel()
	repo, _, _ := setupOpsOrigin(t)
	_, err := opsbackup.Snapshot(adapters.New(repo), opsbackup.SnapshotRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--output")
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
