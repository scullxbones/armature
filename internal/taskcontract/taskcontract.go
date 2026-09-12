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
	reGainsCheck = regexp.MustCompile(`(?i)gains(?:\s+a)?\s+check(?:\s*\(?D\d+\)?)?`)
	reArmDoctor  = regexp.MustCompile(`(?i)\barm\s+doctor\b`)
	reCheckDn    = regexp.MustCompile(`(?i)\bcheck\s+D\d+\b`)
	reHelperOnly = regexp.MustCompile(`(?i)(?:exported helper|helper-only|not wired)`)
)

// CheckTaskContract reports DoD∩scope violations for one issue. Today it encodes
// doctor.run_wiring: a task DoD that claims `arm doctor` / `gains check Dn`
// must include internal/doctor/doctor.go in Scope (or rewrite the DoD as
// helper-only / not wired). Non-tasks and terminal issues are skipped.
func CheckTaskContract(task Task) []Violation {
	if task.Type != "task" || isTerminal(task.Status) {
		return nil
	}
	if !claimsDoctorRunWiring(task.DefinitionOfDone) {
		return nil
	}
	if reHelperOnly.MatchString(task.DefinitionOfDone) {
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

func claimsDoctorRunWiring(dod string) bool {
	if strings.TrimSpace(dod) == "" {
		return false
	}
	if reGainsCheck.MatchString(dod) {
		return true
	}
	return reArmDoctor.MatchString(dod) && reCheckDn.MatchString(dod)
}

func isTerminal(status string) bool {
	switch status {
	case "done", "merged", "cancelled":
		return true
	default:
		return false
	}
}
