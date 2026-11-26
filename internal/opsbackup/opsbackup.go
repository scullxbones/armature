// Package opsbackup implements durable snapshot / verify / restore-plan for the
// published `_armature` tip.
package opsbackup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/clock"
)

const (
	// opsBranch is the unprotected orphan coordination branch.
	opsBranch = "_armature"
	// AdvertisedRef is the tip name written into the bundle header for restore.
	AdvertisedRef = "refs/heads/_armature"
	AsideRef      = "refs/heads/_armature-recovered"

	// Never nest under refs/remotes/origin/_armature. Git cannot create
	// children while a parent ref exists, and concurrent crons must not
	// share one mutable tip.
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
	// (strict ancestor of current tip). No publish / no aside.
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

type git interface {
	BeginPerRunRef(ns string) (ref string, release func(), err error)
	FetchBranchToRef(branch, dstRef string) error
	ResolveRevision(rev string) (string, error)
	BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error
	BundleVerifyRestorable(bundlePath, wantRef string) error
	RestoreEvidence(
		bundlePath, advertiseRef, opsBranch string,
	) (originMissing bool, originTip, recovered string, originAncOfRecovered, recoveredAncOfOrigin bool, err error)
}

type snapshotResult struct {
	Tip    string
	Bundle string
}

// Only durable names: recovered SHA, input bundle path, optional aside publish
// target. Per-run staging refs are never part of the plan output.
type restorePlan struct {
	Recovered   string
	Bundle      string
	Disposition Disposition
	AsideRef    string
}

// ErrDiverge is returned by PlanRestore when tips diverge (aside only).
var ErrDiverge = errors.New("ops-backup: recovered tip diverges from origin/_armature; publish aside to _armature-recovered and escalate")

// Snapshot refreshes the published tip via a per-run tracking ref (never
// writes refs/remotes/origin/_armature), then writes a bundle that
// advertises refs/heads/_armature. Failed refresh aborts before creating the
// output. Live refs/heads/_armature is never moved. On bundle failure a
// pre-existing --output path is left untouched.
func Snapshot(g git, outputPath string) (snapshotResult, error) {
	out := strings.TrimSpace(outputPath)
	if out == "" {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: --output is required")
	}
	runRef, releaseRun, err := g.BeginPerRunRef(runTrackingNS)
	if err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: run tracking ref: %w", err)
	}
	defer releaseRun()
	if err := g.FetchBranchToRef(opsBranch, runRef); err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: refresh origin/%s failed (fail-closed): %w", opsBranch, err)
	}
	tip, err := g.ResolveRevision(runRef)
	if err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: resolve %s: %w", runRef, err)
	}
	releaseRun()

	tmp, cleanup, err := snapshotTempPath(out)
	if err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: temp output: %w", err)
	}
	defer cleanup()

	if err := g.BundleCreateAdvertised(tmp, tip, AdvertisedRef); err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: bundle: %w", err)
	}
	if err := os.Rename(tmp, out); err != nil {
		return snapshotResult{}, fmt.Errorf("ops-backup snapshot: finalize bundle: %w", err)
	}
	return snapshotResult{Tip: tip, Bundle: out}, nil
}

func snapshotTempPath(finalPath string) (tmp string, cleanup func(), err error) {
	dir := filepath.Dir(finalPath)
	if dir == "" || dir == "." {
		dir = "."
	}
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		return "", func() {}, mkErr
	}
	sweepAgedOutputTemps(dir)
	f, err := os.CreateTemp(dir, ".armature-ops-backup-*.bundle")
	if err != nil {
		return "", func() {}, err
	}
	tmp = f.Name()
	swallowErr(f.Close())
	cleaned := false
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true
		swallowErr(os.Remove(tmp))
	}
	return tmp, cleanup, nil
}

// Age ceiling matches adapters.runLeaseMaxAgeSec (1h). Concurrent live temps are seconds old.
func sweepAgedOutputTemps(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, ".armature-ops-backup-*.bundle"))
	if err != nil {
		return
	}
	now := clock.System()
	for _, p := range matches {
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		if now-info.ModTime().Unix() > 3600 {
			swallowErr(os.Remove(p))
		}
	}
}

// Verify confirms a bundle advertises AdvertisedRef and that its object pack
// is restorable (fetch into a throwaway bare — not header-only list-heads).
func Verify(g git, bundlePath string) error {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return fmt.Errorf("ops-backup verify: bundle path is required")
	}
	if err := g.BundleVerifyRestorable(bundlePath, AdvertisedRef); err != nil {
		return fmt.Errorf("ops-backup verify: %w", err)
	}
	return nil
}

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
// Divergence returns that plan and ErrDiverge. Other git failures return an
// error and an empty plan, not DispositionMissing.
func PlanRestore(g git, bundlePath string) (restorePlan, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return restorePlan{}, fmt.Errorf("ops-backup restore-plan: --from is required")
	}
	if err := Verify(g, bundlePath); err != nil {
		return restorePlan{}, err
	}
	originMissing, originTip, recovered, originAnc, recoveredAnc, err := g.RestoreEvidence(bundlePath, AdvertisedRef, opsBranch)
	if err != nil {
		return restorePlan{}, fmt.Errorf("ops-backup restore-plan: %w", err)
	}
	if recovered == "" {
		return restorePlan{}, fmt.Errorf("ops-backup restore-plan: resolve recovered tip: empty SHA")
	}
	plan := restorePlan{
		Recovered: recovered,
		Bundle:    bundlePath,
	}
	plan.Disposition, plan.AsideRef = classifyRestore(originMissing, originTip, recovered, originAnc, recoveredAnc)
	if plan.Disposition == DispositionDivergeAside {
		return plan, ErrDiverge
	}
	return plan, nil
}

func swallowErr(err error) { _ = err }
