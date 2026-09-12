package contextreport

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	ctxpkg "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
)

// DynamicInvocationPaths are the main-path commands whose json/agent stdout
// is priced from the embedded fixture graph. list, ready, and show are AOC
// envelopes from the shared writers. render-context is the live RenderAgent
// JSON; wrapping it would change the CLI contract.
var DynamicInvocationPaths = []string{"list", "ready", "show", "render-context"}

// AOCEnvelopePaths are the dynamic rows that must be compact
// {count,<payload>,help} objects from Write*Envelope.
var AOCEnvelopePaths = []string{"list", "ready", "show"}

func priceDynamicInvocations(state *materialize.State, index materialize.Index, reader ctxpkg.FileReader) ([]Artifact, []byte, error) {
	listPayload, err := measureList(index)
	if err != nil {
		return nil, nil, err
	}
	readyPayload, err := measureReady(index, state, time.Unix(fixtureReadyNow, 0))
	if err != nil {
		return nil, nil, err
	}
	showPayload, err := measureShow(state)
	if err != nil {
		return nil, nil, err
	}
	renderPayload, bundlePayload, err := measureRenderContext(state, reader)
	if err != nil {
		return nil, nil, err
	}

	artifacts := []Artifact{
		price("list", ClassInvocation, listPayload),
		price("ready", ClassInvocation, readyPayload),
		price("show", ClassInvocation, showPayload),
		price("render-context", ClassInvocation, renderPayload),
	}
	return artifacts, bundlePayload, nil
}

func measureList(index materialize.Index) ([]byte, error) {
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var buf bytes.Buffer
	if err := output.WriteListEnvelope(&buf, output.ListRows(index, ids), nil, false, false); err != nil {
		return nil, fmt.Errorf("render list envelope: %w", err)
	}
	return buf.Bytes(), nil
}

func measureReady(index materialize.Index, state *materialize.State, now time.Time) ([]byte, error) {
	entries := ready.ComputeReady(index, state.Issues, "", now.Unix())
	expired := ready.ExpiredClaims(state.Issues, now)
	var buf bytes.Buffer
	if err := output.WriteReadyEnvelope(&buf, entries, nil, false, expired, "", ""); err != nil {
		return nil, fmt.Errorf("render ready envelope: %w", err)
	}
	return buf.Bytes(), nil
}

func measureShow(state *materialize.State) ([]byte, error) {
	issue, ok := state.Issues[FixtureShowIssue]
	if !ok || issue == nil {
		return nil, fmt.Errorf("fixture issue %s not found", FixtureShowIssue)
	}
	row := output.MarshalIssue(issue)
	trunc := output.TruncateShowIssue(&row)
	var buf bytes.Buffer
	// json/agent show is writeShowEnvelope. FormatSpend is human-only
	// (AOC-S2-T3) and must not be priced on this row.
	if err := output.WriteShowEnvelope(&buf, []string{issue.ID}, []output.IssueJSON{row}, trunc); err != nil {
		return nil, fmt.Errorf("render show envelope: %w", err)
	}
	return buf.Bytes(), nil
}

func measureRenderContext(state *materialize.State, reader ctxpkg.FileReader) (invocation, bundle []byte, err error) {
	assembled, err := ctxpkg.Assemble(FixtureShowIssue, state, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("assemble render-context: %w", err)
	}

	raw, err := ctxpkg.RenderAgent(assembled)
	if err != nil {
		return nil, nil, fmt.Errorf("render render-context bundle: %w", err)
	}
	truncated := ctxpkg.Truncate(assembled, DefaultTokenBudget)
	agent, err := ctxpkg.RenderAgent(truncated)
	if err != nil {
		return nil, nil, fmt.Errorf("render render-context invocation: %w", err)
	}
	return append([]byte(agent), '\n'), []byte(raw), nil
}
