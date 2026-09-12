package doctor

import (
	"fmt"
	"strings"
)

// CheckIDsDocRelPath is the companion registry document TOPTIER-S18-T3 owns.
// Live rows are generated from LiveCheckIDs; reservation rows from OpenReservations.
const CheckIDsDocRelPath = "docs/design/doctor-check-ids.md"

// RenderCheckIDsDoc generates docs/design/doctor-check-ids.md from
// LiveCheckIDs and OpenReservations. Live rows must not be hand-edited.
func RenderCheckIDsDoc() (string, error) {
	return renderCheckIDsDoc(LiveCheckIDs(), OpenReservations())
}

func renderCheckIDsDoc(live []string, reservations []CheckReservation) (string, error) {
	if err := validateReservations(live, reservations); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# Doctor check IDs\n\n")
	b.WriteString("Agent-facing registry of `arm doctor` check IDs.\n")
	b.WriteString("Live IDs come from `doctor.LiveCheckIDs()` (the `Run` path, in order).\n")
	b.WriteString("Open-task reservations come from `doctor.OpenReservations()`.\n")
	b.WriteString("Do not hand-edit the live table: regenerate this file from those functions.\n")
	b.WriteString("The drift test `TestCheckIDsDocMatchesLiveAndReservations_REQ_TOPTIER_S18_T3`\n")
	b.WriteString("fails if they diverge.\n\n")
	b.WriteString("This is an allocation ledger, not the product reference for triggers and\n")
	b.WriteString("remediation — see [validation-codes.md](../validation-codes.md) and\n")
	b.WriteString("[commands.md](../commands.md).\n\n")
	b.WriteString("## Live checks\n\n")
	b.WriteString("Generated from `doctor.LiveCheckIDs()`.\n")
	b.WriteString("`RunChecks` still omits D7 (worker-ID mismatches need the validated ops\n")
	b.WriteString("stream); that is documented, not changed.\n\n")
	b.WriteString("<!-- live-check-ids: generated; do not hand-edit -->\n")
	b.WriteString("| ID |\n")
	b.WriteString("|----|\n")
	for _, id := range live {
		fmt.Fprintf(&b, "| `%s` |\n", id)
	}
	b.WriteString("\n## Open reservations\n\n")
	b.WriteString("Git-tracked in `internal/doctor/reservations.go` (`OpenReservations`).\n")
	b.WriteString("A held story may reserve a future ID here; it must not ship a DoD that\n")
	b.WriteString("claims a live ID. S12 remains held — reserve D11 only; do not implement\n")
	b.WriteString("`TOPTIER-S12-T2`.\n\n")
	b.WriteString("<!-- open-reservations: generated from OpenReservations() -->\n")
	b.WriteString("| ID | Issue | Planned |\n")
	b.WriteString("|----|-------|--------|\n")
	if len(reservations) == 0 {
		b.WriteString("| — | — | _none_ |\n")
	} else {
		for _, r := range reservations {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", r.ID, r.Issue, r.Planned)
		}
	}
	b.WriteString("\n## Allocating a new ID\n\n")
	b.WriteString("1. Read `LiveCheckIDs()` and `OpenReservations()` (or this document after a\n")
	b.WriteString("   green drift test).\n")
	b.WriteString("2. Take the next unused `Dn` that appears in neither table.\n")
	b.WriteString("3. If the check is not yet wired into `Run`, add a reservation row (and put\n")
	b.WriteString("   `internal/doctor/doctor.go` in the task scope, or rewrite the DoD as\n")
	b.WriteString("   helper-only / not wired — see `internal/taskcontract`).\n")
	b.WriteString("4. When the check lands in `Run`, remove the reservation. `LiveCheckIDs`\n")
	b.WriteString("   stays in lockstep with `Run` via `TestLiveCheckIDsMatchesRun_REQ_TOPTIER_S18_T0`.\n")
	return b.String(), nil
}
