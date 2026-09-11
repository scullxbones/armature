package contextreport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	ctxpkg "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/review"
)

// listEntry matches cmd/armature/list.go structured stdout (json/agent).
type listEntry struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Parent    string `json:"parent,omitempty"`
	Title     string `json:"title"`
	Outcome   string `json:"outcome,omitempty"`
	ClaimedBy string `json:"claimed_by,omitempty"`
}

type fixtureGit struct {
	diff        string
	changedFile string
}

func (g fixtureGit) ResolveRevision(rev string) (string, error) {
	switch rev {
	case "base":
		return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
	case "head":
		return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil
	default:
		return rev, nil
	}
}

func (g fixtureGit) DiffRange(_, _ string) (string, error) {
	return g.diff, nil
}

func (g fixtureGit) DiffNameOnlyRange(_, _ string) ([]string, error) {
	return []string{g.changedFile}, nil
}

func fixtureDir(repoRoot string) string {
	return filepath.Join(repoRoot, "internal", "contextreport", "testdata")
}

// Collect inventories fixture-measured structured stdout for main-path CLI
// commands plus the fixture render-context bundle.
func Collect(repoRoot string) (Report, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return Report{}, fmt.Errorf("resolve repository path: %w", err)
	}

	root := fixtureDir(abs)
	graphDir := filepath.Join(root, "graph")
	opsPath := filepath.Join(graphDir, "ops.jsonl")
	workspace := filepath.Join(graphDir, "workspace")
	diffPath := filepath.Join(graphDir, "delivery.diff")

	allOps, err := loadFixtureOps(opsPath)
	if err != nil {
		return Report{}, err
	}

	state := materialize.NewState()
	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			return Report{}, fmt.Errorf("replay fixture op %s %s: %w", op.Type, op.TargetID, err)
		}
	}
	index := state.BuildIndex()

	listPayload, err := measureList(index, state)
	if err != nil {
		return Report{}, err
	}
	readyPayload, err := measureReady(index, state)
	if err != nil {
		return Report{}, err
	}
	showPayload, err := measureShow(state)
	if err != nil {
		return Report{}, err
	}
	renderPayload, bundlePayload, err := measureRenderContext(state, workspace)
	if err != nil {
		return Report{}, err
	}
	reviewPayload, err := measureReview(state, diffPath)
	if err != nil {
		return Report{}, err
	}

	artifacts := []Artifact{
		price("list", ClassInvocation, listPayload),
		price("ready", ClassInvocation, readyPayload),
		price("show", ClassInvocation, showPayload),
		price("render-context", ClassInvocation, renderPayload),
		price("review", ClassInvocation, reviewPayload),
		price("render-context.bundle", ClassBundle, bundlePayload),
	}
	return finalize(artifacts), nil
}

func loadFixtureOps(path string) ([]ops.Op, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is testdata under the repo root
	if err != nil {
		return nil, fmt.Errorf("open fixture ops %s: %w", path, err)
	}

	var all []ops.Op
	for lineNo, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		op, err := ops.ParseLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse fixture ops %s:%d: %w", path, lineNo+1, err)
		}
		all = append(all, op)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("fixture ops %s is empty", path)
	}
	return all, nil
}

func measureList(index materialize.Index, state *materialize.State) ([]byte, error) {
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	entries := make([]listEntry, 0, len(ids))
	for _, id := range ids {
		e := index[id]
		le := listEntry{
			ID:      id,
			Type:    e.Type,
			Status:  e.Status,
			Parent:  e.Parent,
			Title:   e.Title,
			Outcome: e.Outcome,
		}
		if issue := state.Issues[id]; issue != nil {
			le.ClaimedBy = issue.ClaimedBy
		}
		entries = append(entries, le)
	}
	return marshalCLIJSON(entries)
}

func measureReady(index materialize.Index, state *materialize.State) ([]byte, error) {
	entries := ready.ComputeReady(index, state.Issues, "")
	var buf bytes.Buffer
	if err := output.RenderReady(&buf, entries, true); err != nil {
		return nil, fmt.Errorf("render ready payload: %w", err)
	}
	return buf.Bytes(), nil
}

func measureShow(state *materialize.State) ([]byte, error) {
	issue, ok := state.Issues[FixtureShowIssue]
	if !ok || issue == nil {
		return nil, fmt.Errorf("fixture issue %s not found", FixtureShowIssue)
	}
	var buf bytes.Buffer
	// show.go emits JSON only for --format json. --format agent and the
	// non-TTY agent default use human RenderIssue; price that payload.
	if err := output.RenderIssue(&buf, issue, false); err != nil {
		return nil, fmt.Errorf("render show payload: %w", err)
	}
	return buf.Bytes(), nil
}

func measureRenderContext(state *materialize.State, workspace string) (invocation, bundle []byte, err error) {
	reader := &ctxpkg.OSFileReader{Root: workspace}
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

func measureReview(state *materialize.State, diffPath string) ([]byte, error) {
	issue, ok := state.Issues[FixtureShowIssue]
	if !ok || issue == nil {
		return nil, fmt.Errorf("fixture issue %s not found", FixtureShowIssue)
	}
	diff, err := os.ReadFile(diffPath) //nolint:gosec // path is testdata under the repo root
	if err != nil {
		return nil, fmt.Errorf("read fixture delivery diff: %w", err)
	}
	criteria, err := review.ParseAcceptanceCriteria(issue.Acceptance)
	if err != nil {
		return nil, fmt.Errorf("parse fixture acceptance: %w", err)
	}
	git := fixtureGit{diff: string(diff), changedFile: "hello.go"}
	bundle, err := review.Prepare(
		git,
		issue.ID,
		issue.Title,
		issue.DefinitionOfDone,
		issue.Type,
		issue.Outcome,
		issue.Scope,
		criteria,
		"base",
		"head",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("prepare fixture review bundle: %w", err)
	}
	return marshalCLIJSON(bundle)
}

func marshalCLIJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal structured payload: %w", err)
	}
	return append(data, '\n'), nil
}
