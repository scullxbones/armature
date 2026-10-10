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

// DynamicInvocationPaths includes render-context as live RenderAgent JSON;
// wrapping it would change the CLI contract.
var DynamicInvocationPaths = []string{"list", "ready", "show", "render-context"}

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

func readyOutputIssues(entries []ready.ReadyEntry) []output.ReadyIssue {
	rows := make([]output.ReadyIssue, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, output.ReadyIssue{
			ID:                   e.Issue,
			Type:                 e.Type,
			Status:               "open",
			Title:                e.Title,
			Parent:               e.Parent,
			Priority:             e.Priority,
			Scope:                e.Scope,
			EstComplexity:        e.EstComplexity,
			RequiresConfirmation: e.RequiresConfirmation,
			AssignedWorker:       e.AssignedWorker,
		})
	}
	return rows
}

func readyOutputExpired(claims []ready.ExpiredClaimEntry) []output.ExpiredClaim {
	rows := make([]output.ExpiredClaim, 0, len(claims))
	for _, c := range claims {
		rows = append(rows, output.ExpiredClaim{
			ID:                         c.Issue,
			Title:                      c.Title,
			Status:                     c.Status,
			ClaimedBy:                  c.ClaimedBy,
			ClaimedAt:                  c.ClaimedAt,
			LastHeartbeat:              c.LastHeartbeat,
			ClaimTTL:                   c.ClaimTTL,
			LastClaimingWorkerActivity: c.LastClaimingWorkerActivity,
		})
	}
	return rows
}

func readyQueueFacts(index materialize.Index, issues map[string]*materialize.Issue, now int64) map[string]ready.Facts {
	out := make(map[string]ready.Facts, len(index))
	for id, e := range index {
		f := ready.Facts{
			Type:           e.Type,
			Status:         e.Status,
			Parent:         e.Parent,
			Title:          e.Title,
			AssignedWorker: e.AssignedWorker,
			Children:       e.Children,
			BlockedBy:      e.BlockedBy,
			Blocks:         e.Blocks,
		}
		if issue := issues[id]; issue != nil {
			f.Priority = issue.Priority
			f.Scope = issue.Scope
			f.EstComplexity = issue.EstComplexity
			f.Confidence = issue.Provenance.Confidence
			f.ClaimedBy = issue.ClaimedBy
			f.ClaimedAt = issue.ClaimedAt
			f.LastHeartbeat = issue.LastHeartbeat
			f.ClaimTTL = issue.ClaimTTL
			f.LastClaimingWorkerActivity = issue.LastClaimingWorkerActivity
			f.ClaimStale = issue.ClaimStale(now)
		}
		out[id] = f
	}
	return out
}

func readyClaimFacts(issues map[string]*materialize.Issue, now int64) map[string]ready.Facts {
	out := make(map[string]ready.Facts, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		out[id] = ready.Facts{
			Status:                     issue.Status,
			Title:                      issue.Title,
			ClaimedBy:                  issue.ClaimedBy,
			ClaimedAt:                  issue.ClaimedAt,
			LastHeartbeat:              issue.LastHeartbeat,
			ClaimTTL:                   issue.ClaimTTL,
			LastClaimingWorkerActivity: issue.LastClaimingWorkerActivity,
			ClaimStale:                 issue.ClaimStale(now),
		}
	}
	return out
}

func measureReady(index materialize.Index, state *materialize.State, now time.Time) ([]byte, error) {
	nowUnix := now.Unix()
	entries := ready.ComputeReady(readyQueueFacts(index, state.Issues, nowUnix), "")
	expired := ready.ExpiredClaims(readyClaimFacts(state.Issues, nowUnix))
	var buf bytes.Buffer
	if err := output.WriteReadyEnvelope(
		&buf,
		readyOutputIssues(entries),
		nil,
		false,
		readyOutputExpired(expired),
		"",
		"",
	); err != nil {
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
	row, trunc := output.TruncateShowIssue(row)
	var buf bytes.Buffer
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
