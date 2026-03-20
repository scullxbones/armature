package traceability

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
)

// IssueRef is a minimal description of an issue used for coverage computation.
// Callers populate this from their own state representation so that this package
// does not need to import materialize (which would create an import cycle).
type IssueRef struct {
	ID              string
	SourceLinkCount int
}

// Coverage holds traceability metrics for the current materialized state.
type Coverage struct {
	TotalNodes  int      `json:"total_nodes"`
	CitedNodes  int      `json:"cited_nodes"`
	CoveragePct float64  `json:"coverage_pct"`
	Uncited     []string `json:"uncited"`
}

// Compute calculates traceability coverage from a slice of IssueRef values.
// An issue is considered "cited" if its SourceLinkCount > 0.
func Compute(refs []IssueRef) Coverage {
	total := len(refs)
	cited := 0
	var uncited []string

	for _, ref := range refs {
		if ref.SourceLinkCount > 0 {
			cited++
		} else {
			uncited = append(uncited, ref.ID)
		}
	}

	sort.Strings(uncited)

	var pct float64
	if total > 0 {
		pct = float64(cited) / float64(total) * 100.0
	}

	return Coverage{
		TotalNodes:  total,
		CitedNodes:  cited,
		CoveragePct: pct,
		Uncited:     uncited,
	}
}

// Write serializes a Coverage value to the given path as JSON (atomic write via temp file).
func Write(path string, c Coverage) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Read deserializes a Coverage value from the given path.
// If the file does not exist, a zero Coverage is returned with no error.
func Read(path string) (Coverage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Coverage{}, nil
		}
		return Coverage{}, err
	}
	var c Coverage
	if err := json.Unmarshal(data, &c); err != nil {
		return Coverage{}, err
	}
	return c, nil
}
