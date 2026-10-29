package contextreport

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const budgetsRelPath = "internal/contextreport/budgets.json"

var runtimeClassOrder = []string{classInvocation, classBundle}

type budget struct {
	Path        string `json:"path"`
	Class       string `json:"class"`
	MaxBytes    int    `json:"max_bytes"`
	TargetBytes int    `json:"target_bytes"`
}

type budgetFile struct {
	Artifacts []budget `json:"artifacts"`
}

type violation struct {
	Path     string
	Class    string
	Bytes    int
	MaxBytes int
}

type gateError struct {
	OverBudget    []violation
	Missing       []string
	Unknown       []string
	ClassMismatch []string
}

func (e *gateError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("context budget gate failed")
	if len(e.OverBudget) > 0 {
		b.WriteString("\nover budget (grouped by class):")
		for _, class := range e.overBudgetClasses() {
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

func (e *gateError) overBudgetClasses() []string {
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

func (e *gateError) failed() bool {
	return e != nil && (len(e.OverBudget) > 0 ||
		len(e.Missing) > 0 ||
		len(e.Unknown) > 0 ||
		len(e.ClassMismatch) > 0)
}

func loadBudgets(path string) (budgetFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the checked-in budgets file or a test fixture
	if err != nil {
		return budgetFile{}, fmt.Errorf("read context budgets %s: %w", path, err)
	}
	return parseBudgets(data)
}

func parseBudgets(data []byte) (budgetFile, error) {
	var file budgetFile
	if err := json.Unmarshal(data, &file); err != nil {
		return budgetFile{}, fmt.Errorf("parse context budgets: %w", err)
	}
	seen := map[string]struct{}{}
	for i, row := range file.Artifacts {
		if strings.TrimSpace(row.Path) == "" {
			return budgetFile{}, fmt.Errorf("context budgets: artifact %d has empty path", i)
		}
		if strings.TrimSpace(row.Class) == "" {
			return budgetFile{}, fmt.Errorf("context budgets: %s has empty class", row.Path)
		}
		if row.MaxBytes < 0 {
			return budgetFile{}, fmt.Errorf("context budgets: %s max_bytes must be >= 0", row.Path)
		}
		if row.TargetBytes <= 0 {
			return budgetFile{}, fmt.Errorf("context budgets: %s target_bytes must be > 0", row.Path)
		}
		if _, dup := seen[row.Path]; dup {
			return budgetFile{}, fmt.Errorf("context budgets: duplicate path %s", row.Path)
		}
		seen[row.Path] = struct{}{}
	}
	return file, nil
}

func enforce(report Report, budgets budgetFile) error {
	byPath := make(map[string]budget, len(budgets.Artifacts))
	for _, row := range budgets.Artifacts {
		byPath[row.Path] = row
	}

	gate := &gateError{}
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
			gate.OverBudget = append(gate.OverBudget, violation{
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

func explicitTargets(report Report, budgets budgetFile) error {
	byPath := make(map[string]budget, len(budgets.Artifacts))
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

func hasDatedTrimPlan(doc, path string) bool {
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

// make check does not call this. Reviewers use it when a raise is proposed.
// A raise is allowed only in a commit that touches the budgets file alone.
func raisedBudgets(previous, current budgetFile) []string {
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
