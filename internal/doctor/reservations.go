package doctor

import (
	"fmt"
	"regexp"
	"strings"
)

// CheckReservation is a git-tracked claim on a doctor check ID that is not yet
// emitted by Run. Planners allocate against LiveCheckIDs plus these rows so a
// held story cannot collide with a live ID. S12 remains held: TOPTIER-S12-T2
// reserves D11 only and does not implement the check.
type CheckReservation struct {
	ID      string
	Issue   string
	Planned string
}

// openReservations is the source of truth for open-task check-ID claims.
// TOPTIER-S18-T3 documents this alongside LiveCheckIDs; do not implement S12-T2.
var openReservations = []CheckReservation{
	{
		ID:      "D11",
		Issue:   "TOPTIER-S12-T2",
		Planned: "Ops-branch backup / disaster-recovery doctor check (narrow-gaps G2.2). S12 remains held; this row reserves D11 only.",
	},
}

var checkIDPattern = regexp.MustCompile(`^D[1-9][0-9]*$`)

// OpenReservations returns git-tracked doctor check-ID reservations for open
// tasks. The returned slice is a copy.
func OpenReservations() []CheckReservation {
	out := make([]CheckReservation, len(openReservations))
	copy(out, openReservations)
	return out
}

func validateReservations(live []string, reservations []CheckReservation) error {
	liveSet := make(map[string]struct{}, len(live))
	for _, id := range live {
		liveSet[id] = struct{}{}
	}
	seen := make(map[string]string, len(reservations))
	for _, r := range reservations {
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("reservation has empty check ID (issue %q)", r.Issue)
		}
		if !checkIDPattern.MatchString(r.ID) {
			return fmt.Errorf("reservation has invalid check ID %q (issue %q)", r.ID, r.Issue)
		}
		if strings.TrimSpace(r.Issue) == "" {
			return fmt.Errorf("reservation %s has empty issue ID", r.ID)
		}
		if strings.TrimSpace(r.Planned) == "" {
			return fmt.Errorf("reservation %s (%s) has empty planned description", r.ID, r.Issue)
		}
		if other, dup := seen[r.ID]; dup {
			return fmt.Errorf("duplicate reserved check ID %s (%s and %s)", r.ID, other, r.Issue)
		}
		seen[r.ID] = r.Issue
		if _, clash := liveSet[r.ID]; clash {
			return fmt.Errorf("reserved check ID %s (%s) collides with live doctor.LiveCheckIDs", r.ID, r.Issue)
		}
	}
	return nil
}
