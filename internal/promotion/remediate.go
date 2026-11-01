package promotion

import (
	"fmt"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
)

// Node is the minimal issue-graph fact RemediationsPlan needs.
type Node struct {
	Status       string
	Parent       string
	Children     []string
	Base         string // delivery base SHA when known
	Tip          string // delivery tip SHA when known
	ClaimedBy    string // claim holder when claimed/in-progress
	ClaimToken   string // published claim token when held
	ClaimedAt    int64  // lease Since
	LastActivity int64  // max(claimed_at, heartbeat, claiming activity)
	TTLMinutes   int    // recorded claim TTL minutes
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
	TreeRepairGuidance = "tree needs repair: open work under a merged ancestor — reopen the merged ancestor chain or cancel/reparent the open child"
	// MoreStepsFollowGuidance marks descendant promotion steps that do not by
	// themselves clear a done-under-merged (D14) finding.
	MoreStepsFollowGuidance = "more steps follow after tree repair to promote remaining descendants"
	// LegacyTemplateGuidance labels <sha> placeholders as legacy templates
	// (same honesty as promotion kind "legacy").
	LegacyTemplateGuidance = "legacy: template argv — substitute real values; placeholders fail closed"
	FindingHeaderPrefix    = "finding: "
)

// RemediationsPlan walks the issue tree under id and returns a typed Plan
// (viewer unknown — every ClaimedBy holder is treated as foreign without Decide).
func RemediationsPlan(id string, g Graph) Plan {
	return RemediationsPlanOpts(id, g, PlanOpts{})
}

// RemediationsPlanFor is RemediationsPlan with the calling worker's ID.
func RemediationsPlanFor(id string, g Graph, viewer string) Plan {
	return RemediationsPlanOpts(id, g, PlanOpts{Viewer: viewer})
}

// RemediationsPlanOpts builds then filters a Plan. Every printed command argv
// (Options + MoreSteps) must pass opts.Decide before it is returned — same
// refusals as claim/transition when Decide is the CLI predicate.
func RemediationsPlanOpts(id string, g Graph, opts PlanOpts) Plan {
	produce := buildRemediationsPlan
	if opts.Producer != nil {
		produce = opts.Producer
	}
	p := produce(id, g, opts)
	p = filterDecideRefused(g, p, opts)
	p = filterFinishedWorkDestroyingAlternatives(g, p)
	return p
}

// RemediationsForIssue is Plan.RenderNextActions for callers that need []string
// (doctor suggested, MERGED-1 next_actions, sync). Prefer RemediationsPlanFor
// when the calling worker ID is known.
func RemediationsForIssue(id string, g Graph) []string {
	return RemediationsPlan(id, g).RenderNextActions()
}

func buildRemediationsPlan(id string, g Graph, opts PlanOpts) Plan {
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

	if chain := mergedAncestorChain(id, g); len(chain) > 0 {
		return treeRepairPlan(id, chain, node, g, opts)
	}
	return healthyPlan(id, node, g)
}

func treeRepairPlan(id string, mergedChain []string, node Node, g Graph, opts PlanOpts) Plan {
	viewer := opts.Viewer
	// Option A: reopen the FULL merged-ancestor chain rootward-first so nested
	// M1→M2→item never leaves M2 open under still-merged M1.
	var reopenSteps []Step
	for _, anc := range mergedChain {
		reopenSteps = append(reopenSteps, Step{Argv: "arm reopen --issue " + anc, Kind: StepCommand})
	}
	reopen := Alternative{Steps: reopenSteps}

	p := Plan{
		Alternatives: []Alternative{reopen},
		Guidance:     []string{TreeRepairGuidance},
	}

	// Option B: cancel open descendants then self — only when every cancel
	// target passes Decide (CLI owner/TTL gate). Live foreign leases: drop
	// Option B; print honest holder guidance (+ real TTL expiry when known).
	cancelTargets := append(openDescendantsUnder(id, g), id)
	var foreignLive []string
	var cancelSteps []Step
	for _, tid := range cancelTargets {
		argv := "arm transition --issue " + tid + " --to cancelled --force"
		if opts.Decide != nil {
			if err := opts.Decide(argv, g); err != nil {
				if looksForeignLive(g[tid], viewer, opts.Now) {
					foreignLive = append(foreignLive, tid)
				}
				// Expired foreign / other refuse: omit from Option B rather than
				// print a CLI-refused cancel (Decide already matched transition).
				continue
			}
		} else if looksForeignLive(g[tid], viewer, opts.Now) {
			foreignLive = append(foreignLive, tid)
			continue
		}
		cancelSteps = append(cancelSteps, Step{Argv: argv, Kind: StepCommand})
	}
	if len(foreignLive) > 0 {
		p.Guidance = append(p.Guidance, foreignLeaseGuidance(foreignLive, g, opts.Now)...)
	} else if len(cancelSteps) > 0 {
		p.Alternatives = append(p.Alternatives, Alternative{Steps: cancelSteps})
	}

	more := healthyDescendantOrLeafSteps(id, node, g)
	if len(more) > 0 {
		p.MoreSteps = more
	}
	return p
}

// looksForeignLive is a graph-side hint for guidance when Decide is absent;
// production Decide (claim.ForeignLiveLeaseBlocks) is authoritative for print.
func looksForeignLive(node Node, viewer string, now int64) bool {
	switch node.Status {
	case ops.StatusClaimed, ops.StatusInProgress:
	default:
		return false
	}
	holder := strings.TrimSpace(node.ClaimedBy)
	if holder == "" || (viewer != "" && holder == viewer) {
		return false
	}
	if now > 0 && node.LastActivity > 0 {
		ttl := node.TTLMinutes
		if ttl <= 0 {
			ttl = 60
		}
		if now >= node.LastActivity+int64(ttl)*60 {
			return false // expired
		}
	}
	return true
}

func foreignLeaseGuidance(ids []string, g Graph, now int64) []string {
	var out []string
	for _, id := range ids {
		n := g[id]
		holder := strings.TrimSpace(n.ClaimedBy)
		if holder == "" {
			holder = "another worker"
		}
		ttlPart := "owner must release (arm transition --issue " + id + " --to cancelled) then rerun arm doctor"
		if n.LastActivity > 0 {
			ttl := n.TTLMinutes
			if ttl <= 0 {
				ttl = 60
			}
			exp := n.LastActivity + int64(ttl)*60
			if now > 0 && now < exp {
				ttlPart = fmt.Sprintf(
					"owner must release (arm transition --issue %s --to cancelled), "+
						"or wait until unix %d (TTL %dm from last activity %d) "+
						"then arm doctor --fix; then rerun arm doctor",
					id, exp, ttl, n.LastActivity,
				)
			}
		}
		out = append(out, fmt.Sprintf(
			"live %s lease on %s held by %s — %s; arm claim --force only overrides scope overlap, not foreign live leases. "+
				"Left-behind worktree may remain at .worktrees/%s. Prefer Option A (reopen) if the holder is unavailable",
			n.Status, id, holder, ttlPart, id,
		))
	}
	return out
}

// filterDecideRefused drops Alternatives / MoreSteps whose command argv fails
// opts.Decide. MoreSteps are checked on the post-reopen graph (after Option A).
func filterDecideRefused(g Graph, p Plan, opts PlanOpts) Plan {
	decide := opts.Decide
	if decide == nil {
		decide = DefaultDecideOnlyTree(opts.Viewer)
	}
	if len(p.Alternatives) > 0 {
		var kept []Alternative
		for _, alt := range p.Alternatives {
			ok := true
			for _, step := range alt.Steps {
				if step.Kind != StepCommand && step.Kind != StepRequiredInput {
					continue
				}
				if err := decide(step.Argv, g); err != nil {
					ok = false
					break
				}
			}
			if ok {
				kept = append(kept, alt)
			}
		}
		p.Alternatives = kept
	}
	if len(p.MoreSteps) > 0 {
		after := graphAfterPrintedReopens(g, p)
		var kept []Step
		for _, step := range p.MoreSteps {
			if step.Kind == StepCommand || step.Kind == StepRequiredInput {
				if err := decide(step.Argv, after); err != nil {
					continue
				}
			}
			kept = append(kept, step)
		}
		p.MoreSteps = kept
	}
	return p
}

// graphAfterPrintedReopens applies arm reopen steps from Alternatives so
// MoreSteps are Decide-checked in the post-repair graph.
func graphAfterPrintedReopens(g Graph, p Plan) Graph {
	after := cloneGraph(g)
	for _, alt := range p.Alternatives {
		for _, cmd := range alt.RunnableCommands() {
			if !strings.Contains(cmd, "arm reopen ") {
				continue
			}
			issue := IssueFlag(cmd)
			if issue == "" {
				continue
			}
			n := after[issue]
			n.Status = ops.StatusOpen
			n.ClaimedBy = ""
			n.ClaimToken = ""
			after[issue] = n
		}
	}
	return after
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
		return nil
	}
	return out
}

func collectHealthyChildren(children []string, g Graph, seen map[string]bool, out *[]Step) {
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
// delivery Base/Tip provenance is known (required-input, not legacy). Shared by
// doctor suggested, MERGED-1 next_actions, and review --help.
func AssessmentHowTo(issueID, base, tip string) string {
	return fmt.Sprintf(
		"produce assessment.json: arm review prepare --issue %s --base %s --head %s --output bundle.json; "+
			"arm review schema --output conformance-assessment.schema.json; "+
			"author ConformanceAssessment JSON per that schema "+
			"(schema_version, bundle_id + contract/delivery fingerprints from the bundle, criterion results with citations); "+
			"arm review validate --assessment assessment.json --bundle bundle.json; "+
			"then arm review record --issue %s --assessment assessment.json --bundle bundle.json",
		issueID, base, tip, issueID,
	)
}

// missingAssessmentActions is the shared recovery list for MERGED-1 /
// sync missing-assessment (same how-to as doctor required-input). AssessmentHowTo
// appears once — on the required-input Help — never doubled as a separate guidance step.
func missingAssessmentActions(issueID, base, tip string) []string {
	base = strings.TrimSpace(base)
	tip = strings.TrimSpace(tip)
	if base == "" {
		base = "<sha>"
	}
	if tip == "" {
		tip = "<sha>"
	}
	return renderStep(Step{
		Argv: "arm review record --issue " + issueID + " --assessment <assessment.json> --bundle <bundle.json>",
		Kind: StepRequiredInput,
		Help: AssessmentHowTo(issueID, base, tip),
	})
}

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
	chain := mergedAncestorChain(id, g)
	if len(chain) == 0 {
		return ""
	}
	return chain[len(chain)-1]
}

// mergedAncestorChain returns every merged ancestor from rootward to nearest.
func mergedAncestorChain(id string, g Graph) []string {
	seen := map[string]bool{}
	var leafward []string
	for cur := g[id].Parent; cur != ""; cur = g[cur].Parent {
		if seen[cur] {
			break
		}
		seen[cur] = true
		node, ok := g[cur]
		if !ok {
			break
		}
		if node.Status == ops.StatusMerged {
			leafward = append(leafward, cur)
		}
	}
	if len(leafward) == 0 {
		return nil
	}
	out := append([]string(nil), leafward...)
	slices.Reverse(out)
	return out
}

// RenderNextActions flattens Plan for CommandFailure next_actions / doctor
// suggested. Order: Option A/B labels on their own lines, then ordered steps,
// then more-steps. Command lines are plain `arm …` (paste-runnable). Never
// alpha-sorts. Template/required-input Help sits on its own line next to argv.
func (p Plan) RenderNextActions() []string {
	var out []string
	if len(p.Alternatives) >= 2 {
		for i, alt := range p.Alternatives {
			out = append(out, optionLabel(i))
			for _, step := range alt.Steps {
				out = append(out, renderStep(step)...)
			}
		}
	} else if len(p.Alternatives) == 1 {
		for _, step := range p.Alternatives[0].Steps {
			out = append(out, renderStep(step)...)
		}
	}
	for _, g := range p.Guidance {
		g = strings.TrimSpace(g)
		if g == "" || g == TreeRepairGuidance {
			continue // TreeRepairGuidance is folded into doctor explanation
		}
		out = append(out, g)
	}
	if len(p.MoreSteps) > 0 {
		if len(p.Alternatives) >= 2 {
			out = append(out, MoreStepsFollowGuidance)
		}
		for _, step := range p.MoreSteps {
			out = append(out, renderStep(step)...)
		}
	}
	return out
}

// RenderGroupedNextActions prefixes a finding header then the typed plan.
// Doctor uses this per item with no cross-item dedup.
func RenderGroupedNextActions(findingItem string, p Plan) []string {
	header := FindingHeaderPrefix + strings.TrimSpace(findingItem)
	return append([]string{header}, p.RenderNextActions()...)
}

func optionLabel(i int) string {
	if i < 0 || i > 25 {
		return fmt.Sprintf("Option %d:", i+1)
	}
	return fmt.Sprintf("Option %c:", 'A'+i)
}

func renderStep(step Step) []string {
	switch step.Kind {
	case StepGuidance:
		return []string{step.Argv}
	case StepLegacyTemplate:
		help := step.Help
		if help == "" {
			help = LegacyTemplateGuidance
		}
		return []string{help, step.Argv}
	case StepRequiredInput:
		var out []string
		if step.Help != "" {
			out = append(out, step.Help)
		}
		out = append(out, step.Argv)
		return out
	default:
		var out []string
		if step.Help != "" {
			out = append(out, step.Help)
		}
		out = append(out, step.Argv)
		return out
	}
}

// IsOptionLabel reports a standalone Option A:/B: line (not an argv).
func IsOptionLabel(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "Option ") {
		return false
	}
	_, rest, found := strings.Cut(s, ":")
	return found && strings.TrimSpace(rest) == ""
}

// RunnableArgv returns the arm command for a rendered next_actions entry.
// Option labels and guidance return empty — command lines are plain `arm …`.
func RunnableArgv(s string) string {
	s = strings.TrimSpace(s)
	if IsOptionLabel(s) || IsGuidanceRemediation(s) || IsLegacyTemplateRemediation(s) {
		return ""
	}
	if strings.HasPrefix(s, "arm ") {
		return s
	}
	return ""
}

// AlternativeRunnable returns arm argv for one alternative.
func (a Alternative) RunnableCommands() []string {
	var out []string
	for _, step := range a.Steps {
		switch step.Kind {
		case StepCommand, StepRequiredInput:
			out = append(out, step.Argv)
		}
	}
	return out
}

// ExplicitlyTouchedIDs returns issue IDs an alternative reopen/cancels/claims.
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

// IsRunnableArmCommand reports whether s is a plain arm argv line.
func IsRunnableArmCommand(s string) bool {
	return RunnableArgv(s) != ""
}

// IsDeliveryRecordTemplate reports a delivery-record argv with SHA placeholders.
func IsDeliveryRecordTemplate(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	return strings.HasPrefix(cmd, "arm delivery record") && strings.Contains(cmd, "<sha>")
}

// IsLegacyTemplateRemediation reports placeholder recovery labeled like kind "legacy".
func IsLegacyTemplateRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == LegacyTemplateGuidance {
		return true
	}
	return IsDeliveryRecordTemplate(cmd)
}

// IsRequiredInputRemediation reports a review-record assessment placeholder.
func IsRequiredInputRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	return strings.HasPrefix(cmd, "arm review record") &&
		(strings.Contains(cmd, "<assessment.json>") || strings.Contains(cmd, "<bundle.json>"))
}

// IsGuidanceRemediation reports non-argv prose entries in a remediation list.
func IsGuidanceRemediation(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if IsOptionLabel(cmd) {
		return true
	}
	if strings.HasPrefix(cmd, FindingHeaderPrefix) {
		return true
	}
	if strings.HasPrefix(cmd, "produce assessment.json:") {
		return true
	}
	if strings.HasPrefix(cmd, "live ") && strings.Contains(cmd, " lease on ") && strings.Contains(cmd, " held by ") {
		return true
	}
	if strings.HasPrefix(cmd, "live claim held by ") {
		return true
	}
	if strings.HasPrefix(cmd, "in-progress live lease held by ") {
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
// no open/claimed/in-progress work under a merged ancestor.
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
	slices.Sort(out)
	return out
}

// WouldViolateTreeConsistency reports whether placing id into open/claimed/
// in-progress under the current graph would violate the invariant (product
// gate for claim and similar state-changing commands).
func WouldViolateTreeConsistency(id string, g Graph) bool {
	if id == "" || g == nil {
		return false
	}
	return nearestMergedAncestor(id, g) != ""
}

// FinishedWorkDestroyed reports done/merged nodes disturbed relative to before,
// unless the chosen alternative explicitly targeted them.
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
	slices.Sort(out)
	return out
}

// applyReopenCancelEffects applies only reopen/cancel argv effects for the
// FinishedWorkDestroyed product check. Not an option validator — Decide owns
// that — and never invents claim-takeover success.
func applyReopenCancelEffects(g Graph, alt Alternative) Graph {
	after := cloneGraph(g)
	for _, cmd := range alt.RunnableCommands() {
		fields := strings.Fields(cmd)
		var issue string
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "--issue" {
				issue = fields[i+1]
			}
		}
		if issue == "" {
			continue
		}
		n := after[issue]
		switch {
		case strings.Contains(cmd, "arm reopen "):
			n.Status = ops.StatusOpen
			n.ClaimedBy = ""
		case strings.Contains(cmd, "--to cancelled"):
			n.Status = ops.StatusCancelled
			n.ClaimedBy = ""
		}
		after[issue] = n
	}
	return after
}

// filterFinishedWorkDestroyingAlternatives drops Alternatives that would disturb
// done/merged nodes they do not explicitly touch — product caller of
// FinishedWorkDestroyed (paired with Decide; no claim-takeover simulation).
func filterFinishedWorkDestroyingAlternatives(g Graph, p Plan) Plan {
	if len(p.Alternatives) == 0 {
		return p
	}
	var kept []Alternative
	for _, alt := range p.Alternatives {
		after := applyReopenCancelEffects(g, alt)
		if d := FinishedWorkDestroyed(g, after, alt.ExplicitlyTouchedIDs()); len(d) > 0 {
			continue
		}
		kept = append(kept, alt)
	}
	p.Alternatives = kept
	return p
}

func cloneGraph(g Graph) Graph {
	out := make(Graph, len(g))
	for id, n := range g {
		n.Children = append([]string(nil), n.Children...)
		out[id] = n
	}
	return out
}

// FindingID extracts the subject issue id from a doctor D14 item
// ("R -> E" or "RC2 under merged E (open)").
func FindingID(item string) string {
	if id, _, ok := strings.Cut(item, " -> "); ok && id != "" {
		return id
	}
	if id, _, ok := strings.Cut(item, " under merged "); ok && id != "" {
		return id
	}
	return item
}
