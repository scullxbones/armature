// Package taskcontract is the leaf for task DoD∩scope rules that arm validate
// will later emit as Graph Findings (E14 is TOPTIER-S18-T2). It must not import
// validate or doctor so T2 can wire CheckTaskContract without a cycle.
package taskcontract

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scullxbones/armature/internal/scopematch"
)

// RuleDoctorRunWiring is the stable rule id T2 maps onto Graph Finding E14.
const RuleDoctorRunWiring = "doctor.run_wiring"

// DoctorRunWiringPath is the Run()/RunChecks wiring file a doctor-check DoD must own.
const DoctorRunWiringPath = "internal/doctor/doctor.go"

// Task is the subset of an issue CheckTaskContract inspects. Callers (T2) map
// materialize.Issue into this so this package stays a leaf.
type Task struct {
	ID               string
	Type             string
	Status           string
	DefinitionOfDone string
	Scope            []string
}

// Violation is one contract failure. Rule is stable (doctor.run_wiring).
type Violation struct {
	Rule    string
	TaskID  string
	Message string
}

var (
	gainsCheckMatcher    = regexp.MustCompile(`(?i)gains(?:\s+a)?\s+check(?:\s*\(?D\d+\)?|\s+\(next\s+free)`)
	addWireDnMatcher     = regexp.MustCompile(`(?i)(?:adds?|wires?)\s+(?:a\s+)?(?:check\s+)?D\d+(?:\s+into\s+Run)?`)
	armDoctorVerbMatcher = regexp.MustCompile(`(?i)\barm\s+doctor\s+(?:gains|reports|emits|enforces|adds|wires)`)
	helperOnlyMatcher    = regexp.MustCompile(`(?i)(?:helper[-\s]only|not[-\s]wired)`)
	doctorRitualMatcher  = regexp.MustCompile(`(?i)` +
		`(?:run\s+)?arm\s+doctor\s+and\s+arm\s+validate(?:\s+--ci)?(?:\s+before\s+(?:done|merging|merge))?` +
		`|` +
		`(?:run\s+)?arm\s+doctor\s+before\s+(?:done|merging|merge)`)
)

// CheckTaskContract reports DoD∩scope violations for one issue. Today it encodes
// doctor.run_wiring: a task DoD that claims to implement `arm doctor` behavior
// (gains/add/wire check D<n>, or arm doctor + implement verb) must include
// internal/doctor/doctor.go in Scope (or rewrite the DoD as helper-only / not
// wired). Narrative mentions — a README pointer to arm doctor, or a validate
// rule that talks about "DoD claims arm doctor/gains check Dn" — are not
// implement claims. Completion-ritual mentions of arm doctor are not claims.
// Non-tasks and terminal issues are skipped.
func CheckTaskContract(task Task) []Violation {
	if task.Type != "task" || isTerminal(task.Status) {
		return nil
	}
	if !ClaimsDoctorRunWiring(task.DefinitionOfDone) {
		return nil
	}
	if scopematch.Allows(task.Scope, DoctorRunWiringPath) {
		return nil
	}
	return []Violation{{
		Rule:   RuleDoctorRunWiring,
		TaskID: task.ID,
		Message: fmt.Sprintf(
			"%s: %s claims arm doctor / gains check Dn but scope does not include %s",
			RuleDoctorRunWiring, task.ID, DoctorRunWiringPath,
		),
	}}
}

// ClaimsDoctorRunWiring reports whether dod is an implement-claim for arm doctor
// Run wiring. Helper-only and not-wired DoDs are not claims. Completion-ritual
// mentions of arm doctor are not claims.
func ClaimsDoctorRunWiring(dod string) bool {
	if strings.TrimSpace(dod) == "" {
		return false
	}
	if helperOnlyMatcher.MatchString(dod) {
		return false
	}
	if gainsCheckMatcher.MatchString(dod) || addWireDnMatcher.MatchString(dod) {
		return true
	}
	stripped := doctorRitualMatcher.ReplaceAllString(dod, " ")
	return armDoctorVerbMatcher.MatchString(stripped)
}

func isTerminal(status string) bool {
	switch status {
	case "done", "merged", "cancelled":
		return true
	default:
		return false
	}
}
