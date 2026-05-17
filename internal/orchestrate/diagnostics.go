package orchestrate

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/scullxbones/armature/internal/ops"
)

type TimeoutDiagnostics struct {
	ElapsedMs int64  `json:"elapsed_ms"`
	LastPhase string `json:"last_phase"`
	Harness   string `json:"harness"`
	Retries   int    `json:"retries"`
	NextStep  string `json:"next_step"`
	Reason    string `json:"reason"`
}

type RunError struct {
	Cause       error
	Diagnostics TimeoutDiagnostics
}

func (e *RunError) Error() string {
	return fmt.Sprintf("orchestration failed in phase=%s: %v", e.Diagnostics.LastPhase, e.Cause)
}

func (e *RunError) Unwrap() error { return e.Cause }

func buildTimeoutNote(diag TimeoutDiagnostics) (ops.Op, error) {
	raw, err := json.Marshal(diag)
	if err != nil {
		return ops.Op{}, err
	}
	return ops.Op{
		Type:      ops.OpNote,
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Msg: string(raw)},
	}, nil
}
