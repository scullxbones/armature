// Package stats computes derived metrics from the append-only ops log.
// Cost estimates (G1.2) turn recorded token counts into per-story and per-wave
// dollar totals using a configurable per-model rate table.
package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
)

// DefaultModel is the rate-table key used when a usage record has no model.
const DefaultModel = "default"

// DefaultRatesFile is the optional repo-local rate table under the ops worktree.
const DefaultRatesFile = "cost-rates.json"

// Rate is USD charged per million input/output tokens for one model.
type Rate struct {
	InputUSDPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputUSDPerMTok float64 `json:"output_usd_per_mtok"`
}

// RateTable maps model identity to USD-per-MTok rates.
type RateTable map[string]Rate

// Usage is one recorded token observation (outcome or assessment). Zero counts
// are unset and never appear here.
type Usage struct {
	IssueID      string
	Model        string
	InputTokens  int
	OutputTokens int
	Source       string // "outcome" or "assessment"
}

// IssueInfo is the hierarchy/scope view cost aggregation needs.
type IssueInfo struct {
	ID             string
	Type           string
	Parent         string
	PreferredModel string
	Scope          []string
}

// Totals is a summed token count plus dollar estimate.
type Totals struct {
	ID           string   `json:"id"`
	USD          float64  `json:"usd"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	Issues       []string `json:"issues,omitempty"`
}

// Report is the --cost view: per-story and per-wave estimates.
type Report struct {
	Stories []Totals          `json:"stories"`
	Waves   []Totals          `json:"waves"`
	Rates   RateTable         `json:"rates"`
	ByIssue map[string]Totals `json:"-"`
}

type rateFile struct {
	Models map[string]Rate `json:"models"`
}

// DefaultRates returns built-in USD-per-MTok prices used when no table is configured.
// Values are list prices for common 2026-era frontier models, not a billing guarantee.
func DefaultRates() RateTable {
	return RateTable{
		"claude-sonnet-4-5": {InputUSDPerMTok: 3.00, OutputUSDPerMTok: 15.00},
		"claude-haiku-4-5":  {InputUSDPerMTok: 1.00, OutputUSDPerMTok: 5.00},
		"claude-opus-4-5":   {InputUSDPerMTok: 15.00, OutputUSDPerMTok: 75.00},
		"gpt-4.1":           {InputUSDPerMTok: 2.00, OutputUSDPerMTok: 8.00},
		"gpt-4.1-mini":      {InputUSDPerMTok: 0.40, OutputUSDPerMTok: 1.60},
		DefaultModel:        {InputUSDPerMTok: 3.00, OutputUSDPerMTok: 15.00},
	}
}

// LoadRateTable reads a JSON object {"models": {name: {input_usd_per_mtok, output_usd_per_mtok}}}.
// Unknown models still fall back to the "default" entry after merge with DefaultRates.
func LoadRateTable(path string) (RateTable, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read rate table: %w", err)
	}
	var parsed rateFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse rate table: %w", err)
	}
	rates := DefaultRates()
	for name, rate := range parsed.Models {
		rates[name] = rate
	}
	return rates, nil
}

// ResolveRates returns flag path, else IssuesDir/cost-rates.json if present, else defaults.
func ResolveRates(ratesPath, issuesDir string) (RateTable, error) {
	if ratesPath != "" {
		return LoadRateTable(ratesPath)
	}
	if issuesDir != "" {
		candidate := filepath.Join(issuesDir, DefaultRatesFile)
		if _, err := os.Stat(candidate); err == nil {
			return LoadRateTable(candidate)
		}
	}
	return DefaultRates(), nil
}

// LoadOps parses every JSONL ops log under opsDir using the same validated
// op stream as snapshot materialization. Ops whose worker_id does not match
// the filename are excluded. Unreadable logs return an error rather than a
// silently understated spend total.
//
// Command handlers that already have a Snapshot must use snap.Ops instead of
// calling LoadOps, so spend and hierarchy share one captured op set.
func LoadOps(opsDir string) ([]ops.Op, error) {
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops logs: %w", err)
	}
	return ops.ExtractOps(items), nil
}

// CollectUsage extracts token-bearing outcome and assessment records. Zero is unset.
// Assessment attestations are deduplicated by issue ID + ResultFingerprint, matching
// materialize.applyAssessmentAttested, so concurrent idempotent appends are billed once.
func CollectUsage(opList []ops.Op) []Usage {
	var out []Usage
	seenAttestation := map[attestationKey]struct{}{}
	for _, op := range opList {
		switch op.Type {
		case ops.OpTransition:
			if op.Payload.InputTokens == 0 && op.Payload.OutputTokens == 0 {
				continue
			}
			out = append(out, Usage{
				IssueID:      op.TargetID,
				Model:        op.Payload.PreferredModel,
				InputTokens:  op.Payload.InputTokens,
				OutputTokens: op.Payload.OutputTokens,
				Source:       "outcome",
			})
		case ops.OpAssessmentAttested:
			var att review.AssessmentAttestation
			if err := json.Unmarshal(op.Payload.Assessment, &att); err != nil {
				continue
			}
			key := attestationKey{issueID: op.TargetID, fingerprint: att.ResultFingerprint}
			if _, dup := seenAttestation[key]; dup {
				continue
			}
			seenAttestation[key] = struct{}{}
			if att.InputTokens == 0 && att.OutputTokens == 0 {
				continue
			}
			out = append(out, Usage{
				IssueID:      op.TargetID,
				Model:        att.ModelIdentity,
				InputTokens:  att.InputTokens,
				OutputTokens: att.OutputTokens,
				Source:       "assessment",
			})
		}
	}
	return out
}

type attestationKey struct {
	issueID     string
	fingerprint string
}

// USDFromTokens converts token counts to dollars using the model's rate.
func USDFromTokens(inputTokens, outputTokens int, rate Rate) float64 {
	const million = 1_000_000.0
	return float64(inputTokens)/million*rate.InputUSDPerMTok +
		float64(outputTokens)/million*rate.OutputUSDPerMTok
}

// RateFor returns the rate for model, then issue preferred model, then default.
func RateFor(table RateTable, model, preferred string) Rate {
	if table == nil {
		table = DefaultRates()
	}
	for _, key := range []string{model, preferred, DefaultModel} {
		if key == "" {
			continue
		}
		if rate, ok := table[key]; ok {
			return rate
		}
	}
	return table[DefaultModel]
}

// Estimate aggregates usage into per-issue, per-story, and per-wave dollar totals.
func Estimate(usages []Usage, issues map[string]IssueInfo, rates RateTable) Report {
	if rates == nil {
		rates = DefaultRates()
	}
	byIssue := make(map[string]Totals)
	for _, u := range usages {
		info := issues[u.IssueID]
		preferred := ""
		if u.Source != "assessment" {
			preferred = info.PreferredModel
		}
		rate := RateFor(rates, u.Model, preferred)
		cur := byIssue[u.IssueID]
		cur.ID = u.IssueID
		cur.InputTokens += u.InputTokens
		cur.OutputTokens += u.OutputTokens
		cur.USD += USDFromTokens(u.InputTokens, u.OutputTokens, rate)
		byIssue[u.IssueID] = cur
	}

	storyBuckets := make(map[string]Totals)
	for issueID, tot := range byIssue {
		root := StoryRoot(issueID, issues)
		bucket := storyBuckets[root]
		bucket.ID = root
		bucket.InputTokens += tot.InputTokens
		bucket.OutputTokens += tot.OutputTokens
		bucket.USD += tot.USD
		storyBuckets[root] = bucket
	}

	report := Report{
		Stories: sortedTotals(storyBuckets),
		Waves:   partitionWaves(byIssue, issues),
		Rates:   rates,
		ByIssue: byIssue,
	}
	return report
}

// StoryRoot walks parents until a story (or the top-most ancestor).
func StoryRoot(issueID string, issues map[string]IssueInfo) string {
	seen := map[string]bool{}
	cur := issueID
	last := issueID
	for cur != "" && !seen[cur] {
		seen[cur] = true
		info, ok := issues[cur]
		if !ok {
			return last
		}
		last = cur
		if info.Type == "story" {
			return cur
		}
		if info.Parent == "" {
			return cur
		}
		cur = info.Parent
	}
	return last
}

// Under reports whether issueID is ancestor or equals rootID.
func Under(issueID, rootID string, issues map[string]IssueInfo) bool {
	seen := map[string]bool{}
	for cur := issueID; cur != "" && !seen[cur]; cur = issues[cur].Parent {
		seen[cur] = true
		if cur == rootID {
			return true
		}
		if _, ok := issues[cur]; !ok {
			return false
		}
	}
	return false
}

// Rollup sums spend for rootID and every descendant.
func Rollup(report Report, rootID string, issues map[string]IssueInfo) Totals {
	out := Totals{ID: rootID}
	for id, tot := range report.ByIssue {
		if Under(id, rootID, issues) {
			out.InputTokens += tot.InputTokens
			out.OutputTokens += tot.OutputTokens
			out.USD += tot.USD
		}
	}
	return out
}

// FormatSpend is the human adjunct line used by arm show and stats.
func FormatSpend(t Totals) string {
	return fmt.Sprintf("Spend-to-date: %s (%d in / %d out)", FormatUSD(t.USD), t.InputTokens, t.OutputTokens)
}

// FormatUSD renders a dollar estimate with six decimal places.
func FormatUSD(usd float64) string {
	return fmt.Sprintf("$%.6f", usd)
}

func sortedTotals(buckets map[string]Totals) []Totals {
	ids := make([]string, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Totals, 0, len(ids))
	for _, id := range ids {
		out = append(out, buckets[id])
	}
	return out
}

func partitionWaves(byIssue map[string]Totals, issues map[string]IssueInfo) []Totals {
	ids := make([]string, 0, len(byIssue))
	for id := range byIssue {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var waves [][]string
	for _, candidate := range ids {
		placed := false
		for i := range waves {
			if canJoinWave(candidate, waves[i], issues) {
				waves[i] = append(waves[i], candidate)
				placed = true
				break
			}
		}
		if !placed {
			waves = append(waves, []string{candidate})
		}
	}

	out := make([]Totals, 0, len(waves))
	for i, members := range waves {
		out = append(out, sumMembers(members, byIssue, i+1))
	}
	return out
}

func sumMembers(members []string, byIssue map[string]Totals, waveN int) Totals {
	tot := Totals{ID: fmt.Sprintf("wave-%d", waveN), Issues: append([]string(nil), members...)}
	for _, id := range members {
		item := byIssue[id]
		tot.InputTokens += item.InputTokens
		tot.OutputTokens += item.OutputTokens
		tot.USD += item.USD
	}
	return tot
}

func canJoinWave(candidate string, wave []string, issues map[string]IssueInfo) bool {
	cand := issues[candidate]
	for _, existing := range wave {
		ex := issues[existing]
		if claim.ScopesOverlap(cand.Scope, ex.Scope) {
			return false
		}
		if related(candidate, existing, issues) {
			return false
		}
	}
	return true
}

func related(a, b string, issues map[string]IssueInfo) bool {
	return Under(a, b, issues) || Under(b, a, issues)
}
