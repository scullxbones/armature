package promotion

import (
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
)

// Node is the minimal issue-graph fact RemediationsPlan needs.
type Node struct {
	Status   string
	Parent   string
	Children []string
	Base     string // delivery base SHA when known
	Tip      string // delivery tip SHA when known
}

// Graph maps issue ID → status/children/parent/delivery. Callers project from
// index/issues; promotion never imports materialize.
type Graph map[string]Node

// StepKind classifies a remediation step for agents and harnesses.
type StepKind string

const (
	// StepCommand is a runnable arm argv.
	StepCommand StepKind = "command"
	// StepGuidance is non-argv prose.
	StepGuidance StepKind = "guidance"
	// StepLegacyTemplate is a placeholder argv with no provenance (kind "legacy").
	StepLegacyTemplate StepKind = "legacy-template"
	// StepRequiredInput is a real gate that needs a produced artifact (not legacy).
	StepRequiredInput StepKind = "required-input"
)

// Step is one ordered action inside an Alternative or MoreSteps.
type Step struct {
	Argv string
	Kind StepKind
	Help string // template label or how-to for required-input; empty for plain commands
}

// Alternative is one mutually exclusive repair path (ordered steps).
type Alternative struct {
	Steps []Step
}

// Plan is the typed repair model shared by doctor suggested and MERGED-1
// next_actions. Alternatives are exclusive choices; MoreSteps follow after a
// choice (typically the reopen/keep path). Never a flat ambiguous sequence.
type Plan struct {
	Alternatives []Alternative
	MoreSteps    []Step
	Guidance     []string
}

const (
	// FallbackGuidance is shape-agnostic text when no graph is available.
	FallbackGuidance = "promote unmerged descendants first; reserve leaf remediations for leaves only"
	// TreeRepairGuidance explains open/unfinished work under a merged ancestor.
	TreeRepairGuidance = "tree needs repair: open work under a merged ancestor — reopen the merged ancestor or cancel/reparent the open child"
	// MoreStepsFollowGuidance marks descendant promotion steps that do not by
	// themselves clear a done-under-merged (D14) finding.
	MoreStepsFollowGuidance = "more steps follow after tree repair to promote remaining descendants"
	// LegacyTemplateGuidance labels <sha> placeholders as legacy templates
	// (same honesty as promotion kind "legacy").
	LegacyTemplateGuidance = "legacy: template argv — substitute real values; placeholders fail closed"
)

// RemediationsPlan walks the issue tree under id and returns a typed Plan.
// A remediation counts only if running an alternative repairs the finding.
//
// Rules:
//   - pending node under a merged ancestor → Alternatives = reopen ancestor OR
//     cancel/reparent this node; never arm claim; MoreSteps promote descendants
//     as if the ancestor were no longer merged
//   - done unmerged leaf (healthy tree) → delivery record (filled SHAs when
//     known + required-input review record; else legacy <sha> template)
//   - open leaf (healthy tree) → arm claim --issue … --worktree
//   - non-leaf with only-cancelled descendants → cancel OR reopen (RunRollup
//     will not promote: needs ≥1 merged child)
//   - rollup-eligible all-merged descendants → empty (materialize owns it)
//
// Merged/cancelled nodes are skipped. Non-leaves are never suggested as arm merged.
func RemediationsPlan(id string, g Graph) Plan {
	if id == "" || g == nil {
		return Plan{}
	}
	node, ok := g[id]
	if !ok {
		return Plan{}
	}
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return Plan{}
	}

	if anc := nearestMergedAncestor(id, g); anc != "" {
		return treeRepairPlan(id, anc, node, g)
	}
	return healthyPlan(id, node, g)
}

// RemediationsForIssue is Plan.RenderNextActions for callers that need []string
// (doctor suggested, MERGED-1 next_actions, sync). Prefer RemediationsPlan.
func RemediationsForIssue(id string, g Graph) []string {
	return RemediationsPlan(id, g).RenderNextActions()
}

func treeRepairPlan(id, anc string, node Node, g Graph) Plan {
	reopen := Alternative{
		Steps: []Step{{Argv: "arm reopen --issue " + anc, Kind: StepCommand}},
	}
	// Option B cancels this node. Any open/claimed/in-progress descendants under
	// the merged ancestor must be cancelled too — otherwise the tree-consistency
	// invariant (no open work under merged ancestor) still fails after cancel.
	var cancelSteps []Step
	for _, openID := range openDescendantsUnder(id, g) {
		cancelSteps = append(cancelSteps, Step{
			Argv: "arm transition --issue " + openID + " --to cancelled --force",
			Kind: StepCommand,
		})
	}
	cancelSteps = append(cancelSteps, Step{
		Argv: "arm transition --issue " + id + " --to cancelled --force",
		Kind: StepCommand,
	})
	p := Plan{
		Alternatives: []Alternative{reopen, {Steps: cancelSteps}},
		Guidance:     []string{TreeRepairGuidance},
	}
	// MoreSteps describe the reopen/keep path only (promote descendants as if
	// the ancestor were no longer merged). Cancel path does not run these.
	more := healthyDescendantOrLeafSteps(id, node, g)
	if len(more) > 0 {
		p.MoreSteps = more
	}
	return p
}

// openDescendantsUnder lists open/claimed/in-progress descendants of id
// (pre-order, children-slice order) — not done/merged/cancelled.
func openDescendantsUnder(id string, g Graph) []string {
	node, ok := g[id]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	var walk func(string)
	walk = func(cid string) {
		if cid == "" || seen[cid] {
			return
		}
		seen[cid] = true
		n, ok := g[cid]
		if !ok {
			return
		}
		switch n.Status {
		case ops.StatusMerged, ops.StatusCancelled, ops.StatusDone:
			// Still walk children: a done container may hold an open grandchild.
			for _, ch := range n.Children {
				walk(ch)
			}
			return
		case ops.StatusOpen, ops.StatusClaimed, ops.StatusInProgress:
			out = append(out, cid)
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	for _, ch := range node.Children {
		walk(ch)
	}
	return out
}

func healthyPlan(id string, node Node, g Graph) Plan {
	if len(node.Children) > 0 {
		steps := healthyDescendantOrLeafSteps(id, node, g)
		if len(steps) == 0 {
			return strandedPlan(id, node, g)
		}
		return Plan{MoreSteps: steps}
	}
	return Plan{MoreSteps: leafPromotionSteps(id, node)}
}

func strandedPlan(id string, node Node, g Graph) Plan {
	if !onlyCancelledDescendants(id, g) {
		return Plan{}
	}
	cancel := Step{Argv: "arm transition --issue " + id + " --to cancelled --force", Kind: StepCommand}
	switch node.Status {
	case ops.StatusDone, ops.StatusBlocked:
		return Plan{
			Alternatives: []Alternative{
				{Steps: []Step{cancel}},
				{Steps: []Step{{Argv: "arm reopen --issue " + id, Kind: StepCommand}}},
			},
		}
	default:
		return Plan{MoreSteps: []Step{cancel}}
	}
}

func healthyDescendantOrLeafSteps(id string, node Node, g Graph) []Step {
	if len(node.Children) == 0 {
		return leafPromotionSteps(id, node)
	}
	seen := map[string]bool{}
	var out []Step
	collectHealthyChildren(node.Children, g, seen, &out)
	if len(out) == 0 {
		// Stranded container handled by caller via strandedPlan.
		return nil
	}
	return out
}

func collectHealthyChildren(children []string, g Graph, seen map[string]bool, out *[]Step) {
	// Preserve model/create order — do not alpha-sort.
	for _, childID := range children {
		collectHealthyNode(childID, g, seen, out)
	}
}

func collectHealthyNode(id string, g Graph, seen map[string]bool, out *[]Step) {
	if id == "" || seen[id] {
		return
	}
	seen[id] = true
	node, ok := g[id]
	if !ok {
		return
	}
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return
	}
	if len(node.Children) > 0 {
		before := len(*out)
		collectHealthyChildren(node.Children, g, seen, out)
		if len(*out) == before && onlyCancelledDescendants(id, g) {
			// Nested stranded under a reopen path: emit cancel (single path —
			// reopen of this container is a separate exclusive choice at its own Plan).
			*out = append(*out, Step{
				Argv: "arm transition --issue " + id + " --to cancelled --force",
				Kind: StepCommand,
			})
		}
		return
	}
	*out = append(*out, leafPromotionSteps(id, node)...)
}

func leafPromotionSteps(id string, node Node) []Step {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	case ops.StatusDone:
		return doneLeafSteps(id, node)
	default:
		return []Step{{Argv: "arm claim --issue " + id + " --worktree", Kind: StepCommand}}
	}
}

func doneLeafSteps(id string, node Node) []Step {
	base := strings.TrimSpace(node.Base)
	tip := strings.TrimSpace(node.Tip)
	if base != "" && tip != "" {
		return []Step{
			{Argv: delivery.RecordArgvFilled(id, base, tip), Kind: StepCommand},
			{
				Argv: "arm review record --issue " + id + " --assessment <assessment.json> --bundle <bundle.json>",
				Kind: StepRequiredInput,
				Help: AssessmentHowTo(id, base, tip),
			},
		}
	}
	return []Step{
		{Argv: delivery.RecordArgv(id), Kind: StepLegacyTemplate, Help: LegacyTemplateGuidance},
	}
}

// AssessmentHowTo is the concrete how-to for producing assessment.json when
// delivery Base/Tip provenance is known (required-input, not legacy).
func AssessmentHowTo(issueID, base, tip string) string {
	return fmt.Sprintf(
		"produce assessment.json: arm review prepare --issue %s --base %s --head %s --output bundle.json; "+
			"author ConformanceAssessment JSON per docs/schemas/conformance-assessment.schema.json "+
			"(schema_version, bundle_id + contract/delivery fingerprints from the bundle, criterion results with citations); "+
			"arm review validate --assessment assessment.json --bundle bundle.json; "+
			"then arm review record --issue %s --assessment assessment.json --bundle bundle.json",
		issueID, base, tip, issueID,
	)
}

// onlyCancelledDescendants reports that id has children and every descendant
// path ends in cancelled work — no merged child and no pending leaf. Unknown
// nodes fail closed (not only-cancelled).
func onlyCancelledDescendants(id string, g Graph) bool {
	node, ok := g[id]
	if !ok || len(node.Children) == 0 {
		return false
	}
	seen := map[string]bool{}
	sawCancelled := false
	var walk func(string) bool
	walk = func(cid string) bool {
		if cid == "" || seen[cid] {
			return true
		}
		seen[cid] = true
		n, ok := g[cid]
		if !ok {
			return false
		}
		switch n.Status {
		case ops.StatusMerged:
			return false
		case ops.StatusCancelled:
			sawCancelled = true
			return true
		}
		if len(n.Children) == 0 {
			return false
		}
		for _, ch := range n.Children {
			if !walk(ch) {
				return false
			}
		}
		return true
	}
	for _, ch := range node.Children {
		if !walk(ch) {
			return false
		}
	}
	return sawCancelled
}

func nearestMergedAncestor(id string, g Graph) string {
	seen := map[string]bool{}
	for cur := g[id].Parent; cur != ""; cur = g[cur].Parent {
		if seen[cur] {
			return ""
		}
		seen[cur] = true
		node, ok := g[cur]
		if !ok {
			return ""
		}
		if node.Status == ops.StatusMerged {
			return cur
		}
	}
	return ""
}

// RenderNextActions flattens Plan for CommandFailure next_actions / doctor
// suggested. Order: Option A/B… (when ≥2 alternatives), then more-steps marker
// + MoreSteps. Never alpha-sorts. Template/required-input Help sits next to its argv.
func (p Plan) RenderNextActions() []string {
	var out []string
	if len(p.Alternatives) >= 2 {
		for i, alt := range p.Alternatives {
			label := optionLabel(i)
			for _, step := range alt.Steps {
				// Every step of an Option keeps the label so agents never see a
				// bare cancel/reopen as a flat sequential next_actions entry.
				out = append(out, renderStep(step, true, label)...)
			}
		}
	} else if len(p.Alternatives) == 1 {
		for _, step := range p.Alternatives[0].Steps {
			out = append(out, renderStep(step, false, "")...)
		}
	}
	if len(p.MoreSteps) > 0 {
		if len(p.Alternatives) >= 2 {
			out = append(out, MoreStepsFollowGuidance)
		}
		for _, step := range p.MoreSteps {
			out = append(out, renderStep(step, false, "")...)
		}
	}
	return out
}

func optionLabel(i int) string {
	if i < 0 || i > 25 {
		return fmt.Sprintf("Option %d:", i+1)
	}
	return fmt.Sprintf("Option %c:", 'A'+i)
}

func renderStep(step Step, withOption bool, label string) []string {
	switch step.Kind {
	case StepGuidance:
		return []string{step.Argv}
	case StepLegacyTemplate:
		var out []string
		help := step.Help
		if help == "" {
			help = LegacyTemplateGuidance
		}
		out = append(out, help)
		argv := step.Argv
		if withOption && label != "" {
			argv = label + " " + argv
		}
		out = append(out, argv)
		return out
	case StepRequiredInput:
		var out []string
		if step.Help != "" {
			out = append(out, step.Help)
		}
		argv := step.Argv
		if withOption && label != "" {
			argv = label + " " + argv
		}
		out = append(out, argv)
		return out
	default:
		argv := step.Argv
		if withOption && label != "" {
			argv = label + " " + argv
		}
		return []string{argv}
	}
}

// OptionArgv strips an "Option X: " prefix; ok is false when absent.
func OptionArgv(s string) (argv string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "Option ") {
		return "", false
	}
	_, rest, found := strings.Cut(s, ":")
	if !found {
		return "", false
	}
	argv = strings.TrimSpace(rest)
	if argv == "" {
		return "", false
	}
	return argv, true
}

// RunnableArgv returns the arm command embedded in a rendered next_actions
// entry (stripping Option labels). Empty when the entry is not runnable.
func RunnableArgv(s string) string {
	s = strings.TrimSpace(s)
	if argv, ok := OptionArgv(s); ok {
		s = argv
	}
	if IsGuidanceRemediation(s) || IsLegacyTemplateRemediation(s) {
		return ""
	}
	if strings.HasPrefix(s, "arm ") {
		return s
	}
	return ""
}

// AlternativeRunnable returns arm argv for one alternative (no Option prefix).
func (a Alternative) RunnableCommands() []string {
	var out []string
	for _, step := range a.Steps {
		switch step.Kind {
		case StepCommand:
			out = append(out, step.Argv)
		case StepRequiredInput:
			// Harness must produce the artifact then run argv; still listed.
			out = append(out, step.Argv)
		}
	}
	return out
}

// ExplicitlyTouchedIDs returns issue IDs an alternative reopen/cancels.
func (a Alternative) ExplicitlyTouchedIDs() map[string]bool {
	out := map[string]bool{}
	for _, cmd := range a.RunnableCommands() {
		fields := strings.Fields(cmd)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "--issue" {
				out[fields[i+1]] = true
			}
		}
	}
	return out
}

// FirstRemediation picks sync's singular next_action from a rendered list.
func FirstRemediation(actions []string) string {
	for _, a := range actions {
		if cmd := RunnableArgv(a); cmd != "" {
			return cmd
		}
	}
	return "arm doctor"
}

// IsRunnableArmCommand reports whether s looks like an arm argv (not prose /
// Option-only / guidance). Option-prefixed lines are runnable after strip.
func IsRunnableArmCommand(s string) bool {
	return RunnableArgv(s) != ""
}

// IsDeliveryRecordTemplate reports a delivery-record argv that still has SHA placeholders.
func IsDeliveryRecordTemplate(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if argv, ok := OptionArgv(cmd); ok {
		cmd = argv
	}
	return strings.HasPrefix(cmd, "arm delivery record") && strings.Contains(cmd, "<sha>")
}

// IsLegacyTemplateRemediation reports placeholder recovery labeled like kind "legacy".
// Required-input assessment steps with known provenance are NOT legacy.
func IsLegacyTemplateRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if argv, ok := OptionArgv(cmd); ok {
		cmd = argv
	}
	if cmd == LegacyTemplateGuidance {
		return true
	}
	if IsDeliveryRecordTemplate(cmd) {
		return true
	}
	// Bare assessment placeholder without a preceding how-to is treated as
	// template only when Help was not rendered (legacy path). Filled-delivery
	// required-input always pairs Help + argv; callers use Step.Kind.
	return false
}

// IsRequiredInputRemediation reports a review-record assessment placeholder
// that is required input (full provenance), not a legacy template.
func IsRequiredInputRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if argv, ok := OptionArgv(cmd); ok {
		cmd = argv
	}
	return strings.HasPrefix(cmd, "arm review record") &&
		(strings.Contains(cmd, "<assessment.json>") || strings.Contains(cmd, "<bundle.json>"))
}

// IsGuidanceRemediation reports non-argv prose entries in a remediation list.
func IsGuidanceRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "Option ") && !strings.Contains(cmd, "arm ") {
		return true
	}
	if strings.HasPrefix(cmd, "produce assessment.json:") {
		return true
	}
	switch cmd {
	case FallbackGuidance, TreeRepairGuidance, MoreStepsFollowGuidance, LegacyTemplateGuidance:
		return true
	default:
		return false
	}
}

// TreeConsistencyViolations reports graph nodes that break the invariant:
// no open/claimed/in-progress work under a merged ancestor. Merged status is
// never "fixed" here — callers separately forbid un-merging.
func TreeConsistencyViolations(g Graph) []string {
	if g == nil {
		return nil
	}
	var out []string
	for id, node := range g {
		switch node.Status {
		case ops.StatusOpen, ops.StatusClaimed, ops.StatusInProgress:
			if anc := nearestMergedAncestor(id, g); anc != "" {
				out = append(out, id+" under merged "+anc+" ("+node.Status+")")
			}
		}
	}
	return out
}

// FinishedWorkDestroyed reports done/merged nodes that were disturbed relative
// to before, unless the chosen alternative explicitly targeted them (cancel or
// reopen). Merged→non-merged without an explicit reopen is always a violation.
func FinishedWorkDestroyed(before, after Graph, explicitlyTouched map[string]bool) []string {
	if before == nil || after == nil {
		return nil
	}
	var out []string
	for id, b := range before {
		a, ok := after[id]
		if !ok {
			continue
		}
		switch b.Status {
		case ops.StatusMerged:
			if a.Status != ops.StatusMerged && !explicitlyTouched[id] {
				out = append(out, id+" un-merged ("+b.Status+"→"+a.Status+")")
			}
		case ops.StatusDone:
			if a.Status != ops.StatusDone && a.Status != ops.StatusMerged && !explicitlyTouched[id] {
				out = append(out, id+" finished work destroyed ("+b.Status+"→"+a.Status+")")
			}
		}
	}
	return out
}
