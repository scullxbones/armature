package contextreport

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// BudgetsRelPath is the repo-relative location of the checked-in per-artifact budgets.
const BudgetsRelPath = "internal/contextreport/budgets.json"

var runtimeClassOrder = []string{ClassInvocation, ClassBundle}

// Budget is one runtime artifact's enforceable cap and named customer promise.
// MaxBytes is what make check enforces. TargetBytes is the promise, not today's size.
type Budget struct {
	Path        string `json:"path"`
	Class       string `json:"class"`
	MaxBytes    int    `json:"max_bytes"`
	TargetBytes int    `json:"target_bytes"`
}

// BudgetFile is the checked-in budget list. Budgets are per runtime artifact.
type BudgetFile struct {
	Artifacts []Budget `json:"artifacts"`
}

// Violation is one artifact whose measured bytes exceed its enforceable cap.
type Violation struct {
	Path     string
	Class    string
	Bytes    int
	MaxBytes int
}

// GateError is a failed budget comparison. Over-budget rows are the hard fail.
// Missing, extra, and class-mismatch rows keep the file aligned with Collect.
type GateError struct {
	OverBudget    []Violation
	Missing       []string
	Unknown       []string
	ClassMismatch []string
}

func (e *GateError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("context budget gate failed")
	if len(e.OverBudget) > 0 {
		b.WriteString("\nover budget (grouped by class):")
		for _, class := range e.OverBudgetClasses() {
			fmt.Fprintf(&b, "\n  %s:", class)
			for _, v := range e.OverBudget {
				if v.Class != class {
					continue
				}
				fmt.Fprintf(&b, "\n    %s: %d bytes > budget %d", v.Path, v.Bytes, v.MaxBytes)
			}
		}
	}
	writePathList(&b, "missing budget", e.Missing)
	writePathList(&b, "unknown budget path", e.Unknown)
	writePathList(&b, "class mismatch", e.ClassMismatch)
	return b.String()
}

// OverBudgetClasses returns distinct classes of over-budget artifacts.
func (e *GateError) OverBudgetClasses() []string {
	if e == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, v := range e.OverBudget {
		seen[v.Class] = struct{}{}
	}
	var classes []string
	for _, class := range runtimeClassOrder {
		if _, ok := seen[class]; ok {
			classes = append(classes, class)
		}
	}
	var extra []string
	for class := range seen {
		if !knownRuntimeClass(class) {
			extra = append(extra, class)
		}
	}
	sort.Strings(extra)
	return append(classes, extra...)
}

func knownRuntimeClass(class string) bool {
	for _, c := range runtimeClassOrder {
		if c == class {
			return true
		}
	}
	return false
}

func writePathList(b *strings.Builder, title string, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:", title)
	for _, p := range paths {
		fmt.Fprintf(b, "\n  %s", p)
	}
}

func (e *GateError) failed() bool {
	return e != nil && (len(e.OverBudget) > 0 ||
		len(e.Missing) > 0 ||
		len(e.Unknown) > 0 ||
		len(e.ClassMismatch) > 0)
}

// LoadBudgets reads a budget file from disk.
func LoadBudgets(path string) (BudgetFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the checked-in budgets file or a test fixture
	if err != nil {
		return BudgetFile{}, fmt.Errorf("read context budgets %s: %w", path, err)
	}
	return ParseBudgets(data)
}

// ParseBudgets decodes and validates a budget file.
func ParseBudgets(data []byte) (BudgetFile, error) {
	var file BudgetFile
	if err := json.Unmarshal(data, &file); err != nil {
		return BudgetFile{}, fmt.Errorf("parse context budgets: %w", err)
	}
	seen := map[string]struct{}{}
	for i, row := range file.Artifacts {
		if strings.TrimSpace(row.Path) == "" {
			return BudgetFile{}, fmt.Errorf("context budgets: artifact %d has empty path", i)
		}
		if strings.TrimSpace(row.Class) == "" {
			return BudgetFile{}, fmt.Errorf("context budgets: %s has empty class", row.Path)
		}
		if row.MaxBytes < 0 {
			return BudgetFile{}, fmt.Errorf("context budgets: %s max_bytes must be >= 0", row.Path)
		}
		if row.TargetBytes <= 0 {
			return BudgetFile{}, fmt.Errorf("context budgets: %s target_bytes must be > 0", row.Path)
		}
		if _, dup := seen[row.Path]; dup {
			return BudgetFile{}, fmt.Errorf("context budgets: duplicate path %s", row.Path)
		}
		seen[row.Path] = struct{}{}
	}
	return file, nil
}

// Enforce fails when any measured artifact exceeds its max_bytes cap, lacks a
// budget, has a class that disagrees with the budget row, or when leftover
// budget paths do not correspond to a measured artifact.
func Enforce(report Report, budgets BudgetFile) error {
	byPath := make(map[string]Budget, len(budgets.Artifacts))
	for _, row := range budgets.Artifacts {
		byPath[row.Path] = row
	}

	gate := &GateError{}
	seen := map[string]struct{}{}
	for _, art := range report.Artifacts {
		seen[art.Path] = struct{}{}
		row, ok := byPath[art.Path]
		if !ok {
			gate.Missing = append(gate.Missing, art.Path)
			continue
		}
		if row.Class != art.Class {
			gate.ClassMismatch = append(gate.ClassMismatch,
				fmt.Sprintf("%s: measured %s, budget %s", art.Path, art.Class, row.Class))
		}
		if art.Bytes > row.MaxBytes {
			gate.OverBudget = append(gate.OverBudget, Violation{
				Path:     art.Path,
				Class:    art.Class,
				Bytes:    art.Bytes,
				MaxBytes: row.MaxBytes,
			})
		}
	}
	for _, row := range budgets.Artifacts {
		if _, ok := seen[row.Path]; !ok {
			gate.Unknown = append(gate.Unknown, row.Path)
		}
	}
	sort.Strings(gate.Missing)
	sort.Strings(gate.Unknown)
	sort.Strings(gate.ClassMismatch)
	sort.Slice(gate.OverBudget, func(i, j int) bool {
		oi, oj := runtimeClassIndex(gate.OverBudget[i].Class), runtimeClassIndex(gate.OverBudget[j].Class)
		if oi != oj {
			return oi < oj
		}
		return gate.OverBudget[i].Path < gate.OverBudget[j].Path
	})
	if !gate.failed() {
		return nil
	}
	return gate
}

func runtimeClassIndex(class string) int {
	for i, c := range runtimeClassOrder {
		if c == class {
			return i
		}
	}
	return len(runtimeClassOrder)
}

// ExplicitTargets fails when a budget row has no named promise, or when the
// row is a silent seed at today's measured size (max and target both equal
// measured bytes). That seed-at-measured-only shape is not a customer cap.
func ExplicitTargets(report Report, budgets BudgetFile) error {
	byPath := make(map[string]Budget, len(budgets.Artifacts))
	for _, row := range budgets.Artifacts {
		byPath[row.Path] = row
	}
	var seeded []string
	for _, art := range report.Artifacts {
		row, ok := byPath[art.Path]
		if !ok {
			return fmt.Errorf("explicit targets: missing budget for %s", art.Path)
		}
		if row.TargetBytes <= 0 {
			return fmt.Errorf("explicit targets: %s has no target_bytes promise", art.Path)
		}
		if row.MaxBytes == art.Bytes && row.TargetBytes == art.Bytes {
			seeded = append(seeded, art.Path)
		}
	}
	if len(seeded) == 0 {
		return nil
	}
	sort.Strings(seeded)
	return fmt.Errorf("explicit targets: measured-size-only seed (no named promise): %s",
		strings.Join(seeded, ", "))
}

// HasDatedTrimPlan reports whether doc contains a markdown heading that names
// path with an ISO date: "### YYYY-MM-DD <path> ...". Fenced code examples do
// not count. A table row, the phrase "trim plan", or an unrelated date
// elsewhere in the file is not a plan for that artifact.
func HasDatedTrimPlan(doc, path string) bool {
	if path == "" {
		return false
	}
	inFence := false
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if datedTrimPlanHeading(line, path) {
			return true
		}
	}
	return false
}

func datedTrimPlanHeading(line, path string) bool {
	const prefix = "### "
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	rest := line[len(prefix):]
	if len(rest) < 11 {
		return false
	}
	if _, err := time.Parse("2006-01-02", rest[:10]); err != nil {
		return false
	}
	if rest[10] != ' ' {
		return false
	}
	name := rest[11:]
	return name == path ||
		strings.HasPrefix(name, path+" ") ||
		strings.HasPrefix(name, path+"(")
}

// RaisedBudgets returns paths whose max_bytes grew relative to previous.
// make check does not call this. Reviewers use it when a raise is proposed.
// A raise is allowed only in a commit that touches the budgets file alone.
func RaisedBudgets(previous, current BudgetFile) []string {
	prev := make(map[string]int, len(previous.Artifacts))
	for _, row := range previous.Artifacts {
		prev[row.Path] = row.MaxBytes
	}
	var raised []string
	for _, row := range current.Artifacts {
		old, ok := prev[row.Path]
		if ok && row.MaxBytes > old {
			raised = append(raised, row.Path)
		}
	}
	sort.Strings(raised)
	return raised
}
