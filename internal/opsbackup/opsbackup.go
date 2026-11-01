// Package opsbackup implements durable snapshot / verify / restore-plan for the
// published `_armature` tip. Policy (fail-closed refresh, I2 aside on diverge,
// bare-temp tip advertisement) lives here; cmd/armature is a thin cobra port.
package opsbackup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// OpsBranch is the unprotected orphan coordination branch.
	OpsBranch = "_armature"
	// TrackingRef is the published tip after FetchTrackingRefWithoutMovingHEAD.
	TrackingRef = "refs/remotes/origin/_armature"
	// LiveRef is the local ops branch. Snapshot must never rewind it.
	LiveRef = "refs/heads/_armature"
	// AdvertisedRef is the tip name written into the bundle header for restore.
	AdvertisedRef = "refs/heads/_armature"
	// ScratchRef is the namespace prefix for per-run restore-plan staging refs
	// (refs/remotes/backup/_armature/<nonce>). Never used as a shared tip.
	ScratchRef = "refs/remotes/backup/_armature"
	// AsideBranch is the I2-safe publish target when tips diverge.
	AsideBranch = "_armature-recovered"
	// AsideRef is the full aside ref name.
	AsideRef = "refs/heads/_armature-recovered"
)

// Disposition is the I2-safe publish plan for a recovered tip.
type Disposition int

const (
	// DispositionMissing: shared tip absent — may create origin/_armature.
	DispositionMissing Disposition = iota
	// DispositionFastForward: recovered is a fast-forward of origin/_armature.
	DispositionFastForward
	// DispositionDivergeAside: push only to _armature-recovered; escalate.
	DispositionDivergeAside
)

func (d Disposition) String() string {
	switch d {
	case DispositionMissing:
		return "missing"
	case DispositionFastForward:
		return "fast-forward"
	case DispositionDivergeAside:
		return "diverge→_armature-recovered"
	default:
		return fmt.Sprintf("disposition(%d)", int(d))
	}
}

// Git is the adapter surface opsbackup needs. *adapters.Client satisfies it.
type Git interface {
	FetchTrackingRefWithoutMovingHEAD(branch string) error
	ResolveRevision(rev string) (string, error)
	IsAncestor(anc, desc string) (bool, error)
	BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error
	BundleVerifyRestorable(bundlePath, wantRef string) error
	FetchBundleForce(bundlePath, srcRef, dstRef string) error
	DeleteRef(ref string) error
}

// SnapshotRequest is the input for Snapshot.
type SnapshotRequest struct {
	OutputPath string
}

// SnapshotResult is the durable tip copy produced by Snapshot.
type SnapshotResult struct {
	Tip    string
	Bundle string
}

// RestorePlan is the printed I2 disposition for a recovered tip.
type RestorePlan struct {
	Recovered   string
	Scratch     string
	Disposition Disposition
	AsideRef    string
}

// ErrDiverge is returned by PlanRestore when tips diverge (aside only).
var ErrDiverge = errors.New("ops-backup: recovered tip diverges from origin/_armature; publish aside to _armature-recovered and escalate")

// Snapshot refreshes origin/_armature via the same +refspec as
// FetchTrackingRefWithoutMovingHEAD, then writes a bundle that advertises
// refs/heads/_armature. Failed refresh aborts before creating the output.
// Live refs/heads/_armature is never moved. On bundle failure a pre-existing
// --output path is left untouched (temp file + rename).
func Snapshot(g Git, req SnapshotRequest) (SnapshotResult, error) {
	out := strings.TrimSpace(req.OutputPath)
	if out == "" {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: --output is required")
	}
	if err := g.FetchTrackingRefWithoutMovingHEAD(OpsBranch); err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: refresh origin/%s failed (fail-closed): %w", OpsBranch, err)
	}
	tip, err := g.ResolveRevision(TrackingRef)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: resolve %s: %w", TrackingRef, err)
	}

	tmp, cleanup, err := snapshotTempPath(out)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: temp output: %w", err)
	}
	defer cleanup()

	if err := g.BundleCreateAdvertised(tmp, tip, AdvertisedRef); err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: bundle: %w", err)
	}
	if err := os.Rename(tmp, out); err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: finalize bundle: %w", err)
	}
	return SnapshotResult{Tip: tip, Bundle: out}, nil
}

func snapshotTempPath(finalPath string) (tmp string, cleanup func(), err error) {
	dir := filepath.Dir(finalPath)
	if dir == "" || dir == "." {
		dir = "."
	}
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		return "", func() {}, mkErr
	}
	f, err := os.CreateTemp(dir, ".armature-ops-backup-*.bundle")
	if err != nil {
		return "", func() {}, err
	}
	tmp = f.Name()
	_ = f.Close() //nolint:errcheck // content written by git bundle create
	cleaned := false
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = os.Remove(tmp) //nolint:errcheck // best-effort; rename success leaves tmp gone
	}
	return tmp, cleanup, nil
}

// Verify confirms a bundle advertises AdvertisedRef and that its object pack
// is restorable (fetch into a throwaway bare — not header-only list-heads).
func Verify(g Git, bundlePath string) error {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return fmt.Errorf("ops-backup verify: bundle path is required")
	}
	if err := g.BundleVerifyRestorable(bundlePath, AdvertisedRef); err != nil {
		return fmt.Errorf("ops-backup verify: %w", err)
	}
	return nil
}

// PlanRestore loads a bundle into a per-run scratch ref under ScratchRef/ and
// returns an I2-safe publish plan. Concurrent restore-plan invocations never
// share a mutable staging tip. It refreshes origin/_armature first. An absent
// advertised remote branch is the documented `missing` disposition (not
// fail-closed). Unreachable origin and other fetch failures stay fail-closed.
// On diverge it still returns the plan with DispositionDivergeAside and ErrDiverge.
func PlanRestore(g Git, bundlePath string) (RestorePlan, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: --from is required")
	}
	if err := Verify(g, bundlePath); err != nil {
		return RestorePlan{}, err
	}
	originAbsent := false
	if err := g.FetchTrackingRefWithoutMovingHEAD(OpsBranch); err != nil {
		if isRemoteRefMissing(err) {
			// Reachable origin, deleted/absent refs/heads/_armature → missing.
			// Do not trust a stale local tracking tip for disposition.
			originAbsent = true
		} else {
			return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: refresh origin/%s failed (fail-closed): %w", OpsBranch, err)
		}
	}
	scratch, err := uniqueScratchRef()
	if err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: scratch ref: %w", err)
	}
	defer func() {
		_ = g.DeleteRef(scratch) //nolint:errcheck // best-effort per-run staging cleanup
	}()
	if err := g.FetchBundleForce(bundlePath, AdvertisedRef, scratch); err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: load bundle: %w", err)
	}
	recovered, err := g.ResolveRevision(scratch)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: resolve scratch: %w", err)
	}
	plan := RestorePlan{
		Recovered: recovered,
		Scratch:   scratch,
	}
	if originAbsent {
		plan.Disposition = DispositionMissing
		return plan, nil
	}
	originTip, originErr := g.ResolveRevision(TrackingRef)
	if originErr != nil {
		plan.Disposition = DispositionMissing
		return plan, nil
	}
	if originTip == recovered {
		plan.Disposition = DispositionFastForward
		return plan, nil
	}
	ff, err := g.IsAncestor(originTip, recovered)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: ancestry: %w", err)
	}
	if ff {
		plan.Disposition = DispositionFastForward
		return plan, nil
	}
	plan.Disposition = DispositionDivergeAside
	plan.AsideRef = AsideRef
	return plan, ErrDiverge
}

func uniqueScratchRef() (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return ScratchRef + "/" + hex.EncodeToString(nonce[:]), nil
}

// isRemoteRefMissing mirrors adapters.IsRemoteRefMissing without importing the
// adapter package (opsbackup stays on the Git port). Same class: reachable
// origin that does not advertise the branch, not transport failure.
func isRemoteRefMissing(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "couldn't find remote ref") ||
		strings.Contains(lower, "does not exist in remote") ||
		strings.Contains(lower, "no such ref") ||
		(strings.Contains(lower, "remote ref") && strings.Contains(lower, "not found"))
}
