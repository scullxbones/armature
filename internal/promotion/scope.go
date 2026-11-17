package promotion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

// Scope kinds printed by `arm promotion-ready scope`.
const (
	ScopeKindScoped = "scoped"
	ScopeKindNone   = "no-scope"
)

// AncestorFunc reports whether ancestor is an ancestor of descendant
// (a commit is an ancestor of itself). A missing object is the caller's
// choice; DeriveScope forwards errors.
type AncestorFunc func(ancestor, descendant string) (bool, error)

// ScopeIssue is the ops view DeriveScope needs. It does not read a PR body.
type ScopeIssue struct {
	ID          string
	Status      string
	Base        string
	Tip         string
	Children    []string
	Assessments []AssessmentFact
}

// ScopeInput is the PR-shaped query for ID-scoped deliveries.
// Boundary is the pull-request base SHA (the exclusion edge).
// BoundaryRef is that ref's name (for the stacked log line).
// IntegrationRef is the repo integration branch (default main).
type ScopeInput struct {
	Head           string
	Boundary       string
	BoundaryRef    string
	IntegrationRef string
	Issues         []ScopeIssue
	Ancestor       AncestorFunc
}

// ScopeDelivery is one promotion-ready invocation: leaves that share a
// recorded delivery base. Assessed is true when any leaf has an attestation
// keyed by (Base, Head) — the PR head, not an older recorded tip.
type ScopeDelivery struct {
	Base     string
	IssueIDs []string
	Assessed bool
}

// ScopeResult is the derived delivery set. Kind is scoped or no-scope.
// Stacked is true when the PR base ref is not the integration branch.
type ScopeResult struct {
	Kind       string
	Stacked    bool
	Deliveries []ScopeDelivery
}

// DeriveScope selects done leaves whose recorded tip is in the PR's unique
// history: an ancestor of Head (including Head) and not an ancestor of
// Boundary. Parents are not leaves. --base for each group is the recorded
// delivery base. A tip in range with no recorded base is an error.
func DeriveScope(in ScopeInput) (ScopeResult, error) {
	if strings.TrimSpace(in.Head) == "" {
		return ScopeResult{}, fmt.Errorf("promotion-ready: head SHA is required")
	}
	if strings.TrimSpace(in.Boundary) == "" {
		return ScopeResult{}, fmt.Errorf("promotion-ready: boundary SHA is required")
	}
	if in.Ancestor == nil {
		return ScopeResult{}, fmt.Errorf("promotion-ready: ancestor check is required")
	}
	integration := strings.TrimSpace(in.IntegrationRef)
	if integration == "" {
		integration = "main"
	}
	res := ScopeResult{
		Kind:    ScopeKindNone,
		Stacked: in.BoundaryRef != "" && in.BoundaryRef != integration,
	}

	groups := map[string][]string{}
	assessed := map[string]bool{}
	seen := map[string]bool{}
	for _, iss := range in.Issues {
		if iss.ID == "" || seen[iss.ID] {
			continue
		}
		seen[iss.ID] = true
		if iss.Status != ops.StatusDone || len(iss.Children) > 0 {
			continue
		}
		tip := strings.TrimSpace(iss.Tip)
		if tip == "" {
			continue
		}
		inHead, err := in.Ancestor(tip, in.Head)
		if err != nil {
			return ScopeResult{}, fmt.Errorf("promotion-ready: place tip of %s: %w", iss.ID, err)
		}
		if !inHead {
			continue
		}
		onBoundary, err := in.Ancestor(tip, in.Boundary)
		if err != nil {
			return ScopeResult{}, fmt.Errorf("promotion-ready: place tip of %s: %w", iss.ID, err)
		}
		if onBoundary {
			continue
		}
		base := strings.TrimSpace(iss.Base)
		if base == "" {
			return ScopeResult{}, fmt.Errorf(
				"promotion-ready: %s has tip %s in %s..%s but no recorded delivery base",
				iss.ID, tip, in.Boundary, in.Head,
			)
		}
		groups[base] = append(groups[base], iss.ID)
		if !assessed[base] && hasAssessment(iss.Assessments, base, in.Head) {
			assessed[base] = true
		}
	}
	if len(groups) == 0 {
		return res, nil
	}
	bases := make([]string, 0, len(groups))
	for base := range groups {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	res.Kind = ScopeKindScoped
	res.Deliveries = make([]ScopeDelivery, 0, len(bases))
	for _, base := range bases {
		ids := append([]string(nil), groups[base]...)
		sort.Strings(ids)
		res.Deliveries = append(res.Deliveries, ScopeDelivery{
			Base:     base,
			IssueIDs: ids,
			Assessed: assessed[base],
		})
	}
	return res, nil
}

func hasAssessment(atts []AssessmentFact, base, head string) bool {
	for _, att := range atts {
		if att.BaseSHA == base && att.HeadSHA == head {
			return true
		}
	}
	return false
}

// FormatScope is the log contract for no-scope, stacked, and each delivery.
func FormatScope(in ScopeInput, res ScopeResult) string {
	integration := strings.TrimSpace(in.IntegrationRef)
	if integration == "" {
		integration = "main"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "promotion-scope: kind=%s head=%s boundary=%s boundary_ref=%s stacked=%t integration=%s\n",
		res.Kind, in.Head, in.Boundary, in.BoundaryRef, res.Stacked, integration)
	if res.Kind == ScopeKindNone {
		b.WriteString("promotion-scope: no ID-scoped delivery; pass is not a bypass of R2/R3/R4 " +
			"because arm land promotes only done leaves whose tip is in this range and there are none\n")
		return b.String()
	}
	if res.Stacked {
		fmt.Fprintf(&b, "promotion-scope: stacked base ref %s is not integration %s; pass/fail is still R2/R3/R4 on each recorded delivery base at this head\n",
			in.BoundaryRef, integration)
	}
	for _, d := range res.Deliveries {
		fmt.Fprintf(&b, "promotion-scope: delivery base=%s issues=%s assessed=%t\n",
			d.Base, strings.Join(d.IssueIDs, ","), d.Assessed)
	}
	return b.String()
}
