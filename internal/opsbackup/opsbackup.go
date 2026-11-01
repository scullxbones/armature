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
	// TrackingRef is the conventional published tip name
	// (refs/remotes/origin/_armature). Ops-backup never writes it: snapshot
	// fetches into per-run refs under runTrackingNS via FetchBranchToRef
	// (empty --refmap= so the remote's configured refspec cannot also update
	// this shared tip under concurrency). restore-plan stages only under
	// refs/armature-tmp/<run-id>/ inside the clone (cleaned up; never
	// refs/remotes/origin/*).
	TrackingRef = "refs/remotes/origin/_armature"
	// LiveRef is the local ops branch. Snapshot must never rewind it.
	LiveRef = "refs/heads/_armature"
	// AdvertisedRef is the tip name written into the bundle header for restore.
	AdvertisedRef = "refs/heads/_armature"
	// AsideBranch is the I2-safe publish target when tips diverge.
	AsideBranch = "_armature-recovered"
	// AsideRef is the full aside ref name.
	AsideRef = "refs/heads/_armature-recovered"

	// runTrackingNS is the sibling namespace for per-run origin tip fetches
	// during snapshot. Never nest under TrackingRef — Git cannot create
	// children while a parent ref exists, and concurrent crons must not share
	// one mutable tip.
	runTrackingNS = "refs/remotes/origin/_armature-run"
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
	// DispositionAlreadyContained: recovered tip is already in origin history
	// (strict ancestor of current tip). Exit 0; no publish / no aside.
	DispositionAlreadyContained
)

func (d Disposition) String() string {
	switch d {
	case DispositionMissing:
		return "missing"
	case DispositionFastForward:
		return "fast-forward"
	case DispositionDivergeAside:
		return "diverge→_armature-recovered"
	case DispositionAlreadyContained:
		return "already-contained"
	default:
		return fmt.Sprintf("disposition(%d)", int(d))
	}
}

// Git is the adapter surface opsbackup needs. *adapters.Client satisfies it.
type Git interface {
	FetchBranchToRef(branch, dstRef string) error
	ResolveRevision(rev string) (string, error)
	BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error
	BundleVerifyRestorable(bundlePath, wantRef string) error
	DeleteRef(ref string) error
	// RestoreEvidence compares origin+bundle tips via clone-local network ops
	// and refs/armature-tmp/<run-id>/ staging (cleaned up on return).
	RestoreEvidence(
		bundlePath, advertiseRef, opsBranch string,
	) (originMissing bool, originTip, recovered string, originAncOfRecovered, recoveredAncOfOrigin bool, err error)
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
// Only durable names: recovered SHA, input bundle path, optional aside publish
// target. Per-run staging refs are never part of the plan output.
type RestorePlan struct {
	Recovered   string
	Bundle      string
	Disposition Disposition
	AsideRef    string
}

// ErrDiverge is returned by PlanRestore when tips diverge (aside only).
var ErrDiverge = errors.New("ops-backup: recovered tip diverges from origin/_armature; publish aside to _armature-recovered and escalate")

// Snapshot refreshes the published tip via a per-run tracking ref (sibling of
// TrackingRef; never writes the shared tip), then writes a bundle that
// advertises refs/heads/_armature. Failed refresh aborts before creating the
// output. Live refs/heads/_armature is never moved. On bundle failure a
// pre-existing --output path is left untouched (temp file + rename).
func Snapshot(g Git, req SnapshotRequest) (SnapshotResult, error) {
	out := strings.TrimSpace(req.OutputPath)
	if out == "" {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: --output is required")
	}
	runRef, err := uniqueRunRef(runTrackingNS)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: run tracking ref: %w", err)
	}
	defer func() {
		_ = g.DeleteRef(runRef) //nolint:errcheck // best-effort per-run staging cleanup
	}()
	if err := g.FetchBranchToRef(OpsBranch, runRef); err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: refresh origin/%s failed (fail-closed): %w", OpsBranch, err)
	}
	tip, err := g.ResolveRevision(runRef)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("ops-backup snapshot: resolve %s: %w", runRef, err)
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

// classifyRestore picks an I2 disposition from positive origin evidence and a
// recovered tip. recovered strict ancestor of current origin → already-contained
// (exit 0, no aside): the tip is already in published history. Unrelated tips
// remain diverge→aside.
func classifyRestore(originMissing bool, originTip, recovered string, originAncOfRecovered, recoveredAncOfOrigin bool) (Disposition, string) {
	if originMissing {
		return DispositionMissing, ""
	}
	if originTip == recovered || originAncOfRecovered {
		return DispositionFastForward, ""
	}
	if recoveredAncOfOrigin {
		return DispositionAlreadyContained, ""
	}
	return DispositionDivergeAside, AsideRef
}

// PlanRestore verifies a bundle, then compares its tip to origin/_armature via
// RestoreEvidence (clone-local network ops + per-call refs/armature-tmp staging,
// so overlapping restore-plan runs cannot cross-read each other's refs).
//
// Disposition audit (success only on positive evidence):
//   - missing: ls-remote reports origin/_armature absent (structural)
//   - fast-forward: origin tip resolved and equals recovered, or is an ancestor
//   - already-contained: recovered is a strict ancestor of current origin
//     (exit 0; no aside — tip already in published history)
//   - diverge→_armature-recovered: origin tip resolved and histories are
//     unrelated (aside only; ErrDiverge)
//
// Unreachable origin, bundle/ancestry errors, and other git failures stay
// fail-closed (mapped error, no success disposition). Printed fields are
// durable only (recovered SHA, bundle path, optional aside publish name).
func PlanRestore(g Git, bundlePath string) (RestorePlan, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: --from is required")
	}
	if err := Verify(g, bundlePath); err != nil {
		return RestorePlan{}, err
	}
	originMissing, originTip, recovered, originAnc, recoveredAnc, err := g.RestoreEvidence(bundlePath, AdvertisedRef, OpsBranch)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: %w", err)
	}
	if recovered == "" {
		return RestorePlan{}, fmt.Errorf("ops-backup restore-plan: resolve recovered tip: empty SHA")
	}
	plan := RestorePlan{
		Recovered: recovered,
		Bundle:    bundlePath,
	}
	plan.Disposition, plan.AsideRef = classifyRestore(originMissing, originTip, recovered, originAnc, recoveredAnc)
	if plan.Disposition == DispositionDivergeAside {
		return plan, ErrDiverge
	}
	return plan, nil
}

func uniqueRunRef(ns string) (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return ns + "/" + hex.EncodeToString(nonce[:]), nil
}
