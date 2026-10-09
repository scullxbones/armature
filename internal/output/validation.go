package output

import (
	"fmt"
	"io"
)

// ValidationView is the presentation shape of a graph-validation result.
// Composition-root callers map validate.Result onto this type so output does
// not import the validate module.
type ValidationView struct {
	OK       bool
	Errors   []string
	Warnings []string
	Infos    []string
	Coverage *CoverageSummary
}

// CoverageSummary is the coverage counts RenderValidation/CoverageLine print.
type CoverageSummary struct {
	TotalNodes        int
	CitedNodes        int
	AcceptedRiskNodes int
}

func RenderValidation(w io.Writer, result ValidationView, quiet bool) error {
	for _, e := range result.Errors {
		if _, err := fmt.Fprintf(w, "ERROR: %s\n", e); err != nil {
			return err
		}
	}
	for _, warn := range result.Warnings {
		if _, err := fmt.Fprintf(w, "WARNING: %s\n", warn); err != nil {
			return err
		}
	}
	if !quiet {
		for _, info := range result.Infos {
			if _, err := fmt.Fprintf(w, "INFO: %s\n", info); err != nil {
				return err
			}
		}
	}
	if line := CoverageLine(result); line != "" {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if result.OK {
		if _, err := fmt.Fprintln(w, "OK: no issues found"); err != nil {
			return err
		}
	}
	return nil
}

func CoverageLine(result ValidationView) string {
	if result.Coverage == nil {
		return ""
	}
	cov := result.Coverage
	totalCited := cov.CitedNodes + cov.AcceptedRiskNodes
	if cov.AcceptedRiskNodes > 0 {
		return fmt.Sprintf("COVERAGE: %d/%d cited (%d source-linked, %d accepted-risk)",
			totalCited, cov.TotalNodes, cov.CitedNodes, cov.AcceptedRiskNodes)
	}
	return fmt.Sprintf("COVERAGE: %d/%d cited", totalCited, cov.TotalNodes)
}
