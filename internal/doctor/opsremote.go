package doctor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
)

// localOnlyOpsStaleAfter is the G2.2 / D11 staleness window for unpublished
// local ops commits on _armature. Named package constant — not a config knob.
const localOnlyOpsStaleAfter = 24 * time.Hour

const (
	d11TrackingRef     = "origin/_armature"
	d11RemediationHint = "arm ops-backup snapshot|restore-plan"
)

type d11Observation struct {
	missingTracking bool
	localOnlyCount  int
	localOnlyAge    time.Duration
}

func checkD11OpsRemote(worktreePath string, now time.Time) Finding {
	skip := Finding{Check: "D11", Severity: SeverityOK, Message: "Ops remote tracking not checked"}
	if strings.TrimSpace(worktreePath) == "" {
		return skip
	}
	info, err := os.Stat(worktreePath)
	if err != nil || !info.IsDir() {
		return skip
	}

	gc := adapters.New(worktreePath)
	fetchErr := gc.FetchTrackingRefWithoutMovingHEAD("_armature")
	obs := observeD11OpsRemote(gc, fetchErr, now)
	return evaluateD11OpsRemote(obs)
}

func observeD11OpsRemote(gc *adapters.Client, fetchErr error, now time.Time) d11Observation {
	tipMissing := adapters.IsRemoteRefMissing(fetchErr)
	if !tipMissing {
		if _, err := gc.ResolveRevision(d11TrackingRef); err != nil {
			tipMissing = true
		}
	}
	if tipMissing {
		return d11Observation{missingTracking: true}
	}

	ahead, err := gc.RevListCount(d11TrackingRef + "..HEAD")
	if err != nil || ahead == 0 {
		return d11Observation{}
	}

	localOnly, err := gc.FirstParentAfter(d11TrackingRef, "HEAD")
	if err != nil || len(localOnly) == 0 {
		return d11Observation{localOnlyCount: ahead}
	}
	oldestUnix, err := gc.CommitterUnix(localOnly[0])
	if err != nil {
		return d11Observation{localOnlyCount: ahead}
	}
	age := now.Sub(time.Unix(oldestUnix, 0))
	if age < 0 {
		age = 0
	}
	return d11Observation{
		localOnlyCount: ahead,
		localOnlyAge:   age,
	}
}

func evaluateD11OpsRemote(obs d11Observation) Finding {
	f := Finding{
		Check:    "D11",
		Severity: SeverityOK,
		Message:  "Ops remote tracking tip is verified",
	}
	if obs.missingTracking {
		f.Severity = SeverityWarning
		f.Message = "Ops branch has no verified remote tracking tip (origin/_armature missing)"
		f.Items = []string{d11RemediationHint}
		return f
	}
	if obs.localOnlyCount > 0 && obs.localOnlyAge >= localOnlyOpsStaleAfter {
		f.Severity = SeverityWarning
		f.Message = fmt.Sprintf(
			"Local-only ops on _armature are stale (%d unpublished commit(s) for %s; threshold %s)",
			obs.localOnlyCount,
			obs.localOnlyAge.Round(time.Second),
			localOnlyOpsStaleAfter,
		)
		f.Items = []string{d11RemediationHint}
		return f
	}
	return f
}
