// Package opsbackup implements durable snapshot / verify / restore-plan for the
// published `_armature` tip. Policy (fail-closed refresh, I2 aside on diverge,
// bare-temp tip advertisement) lives here; cmd/armature is a thin cobra port.
package opsbackup

import (
	"errors"
	"fmt"
	"os"
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
	// ScratchRef is the force-updated tracking ref restore-plan loads into.
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
	BundleListHeads(bundlePath string) ([]string, error)
	FetchBundleForce(bundlePath, srcRef, dstRef string) error
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
// Live refs/heads/_armature is never moved.
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
	if err := g.BundleCreateAdvertised(out, tip, AdvertisedRef); err != nil {
		if remErr := os.Remove(out); remErr != nil && !errors.Is(remErr, os.ErrNotExist) {
			return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: bundle: %w (cleanup: %v)", err, remErr)
		}
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: bundle: %w", err)
	}
	return SnapshotResult{Tip: tip, Bundle: out}, nil
}

// Verify confirms a bundle is readable and advertises AdvertisedRef.
func Verify(g Git, bundlePath string) error {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return fmt.Errorf("ops-backup verify: bundle path is required")
	}
	refs, err := g.BundleListHeads(bundlePath)
	if err != nil {
		return fmt.Errorf("ops-backup verify: %w", err)
	}
	for _, ref := range refs {
		if ref == AdvertisedRef {
			return nil
		}
	}
	return fmt.Errorf("ops-backup verify: bundle does not advertise %s (got %v)", AdvertisedRef, refs)
}

// PlanRestore loads a bundle into ScratchRef and returns an I2-safe publish plan.
// It refreshes origin/_armature first (fail-closed). On diverge it still returns
// the plan with DispositionDivergeAside and ErrDiverge.
func PlanRestore(g Git, bundlePath string) (RestorePlan, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: --from is required")
	}
	if err := Verify(g, bundlePath); err != nil {
		return RestorePlan{}, err
	}
	if err := g.FetchTrackingRefWithoutMovingHEAD(OpsBranch); err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: refresh origin/%s failed (fail-closed): %w", OpsBranch, err)
	}
	if err := g.FetchBundleForce(bundlePath, AdvertisedRef, ScratchRef); err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: load bundle: %w", err)
	}
	recovered, err := g.ResolveRevision(ScratchRef)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: resolve scratch: %w", err)
	}
	plan := RestorePlan{
		Recovered: recovered,
		Scratch:   ScratchRef,
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
