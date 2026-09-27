//go:build ignore
// +build ignore

package examples

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCriterionStatus_JSONRoundTrip_REQ_DF_S5_T6(t *testing.T) {
	type CriterionStatus string
	const StatusSatisfied CriterionStatus = "satisfied"

	type CriterionResult struct {
		Status CriterionStatus `json:"status"`
	}

	raw := `{"status":"satisfied"}`
	var result CriterionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Status != StatusSatisfied {
		t.Errorf("got %v, want StatusSatisfied", result.Status)
	}

	out, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(out), `"satisfied"`) {
		t.Errorf("marshal produced %s, want string form with \"satisfied\"", out)
	}
}
