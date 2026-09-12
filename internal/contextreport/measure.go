package contextreport

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
)

//go:embed testdata/graph
var fixtureGraph embed.FS

const (
	fixtureOpsName      = "testdata/graph/ops.jsonl"
	fixtureDiffName     = "testdata/graph/delivery.diff"
	fixtureWorkspaceDir = "testdata/graph/workspace"
	// fixtureReadyNow is after every fixture claim TTL (claimed_at 1700000004, ttl 60)
	// so expired_claims matches live arm ready against this graph.
	fixtureReadyNow = int64(1_800_000_000)
)

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

// embedFileReader reads workspace files from the embedded fixture graph.
type embedFileReader struct{}

func (embedFileReader) ReadFile(relPath string) ([]byte, error) {
	rel := path.Clean(filepath.ToSlash(relPath))
	if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return nil, fmt.Errorf("invalid fixture path %q", relPath)
	}
	return fixtureGraph.ReadFile(path.Join(fixtureWorkspaceDir, rel))
}

// Collect inventories fixture-measured structured stdout for main-path CLI
// commands plus the fixture render-context bundle. Fixtures are embedded in
// the binary so the command does not depend on the --repo tree.
func Collect() (Report, error) {
	state, index, err := replayFixtureState()
	if err != nil {
		return Report{}, err
	}

	dynamic, bundlePayload, err := priceDynamicInvocations(state, index, embedFileReader{})
	if err != nil {
		return Report{}, err
	}
	reviewPayload, err := measureReview(state)
	if err != nil {
		return Report{}, err
	}

	artifacts := append(dynamic,
		price("review", ClassInvocation, reviewPayload),
		price("render-context.bundle", ClassBundle, bundlePayload),
	)
	return finalize(artifacts), nil
}

func replayFixtureState() (*materialize.State, materialize.Index, error) {
	allOps, err := loadEmbeddedOps()
	if err != nil {
		return nil, nil, err
	}
	state := materialize.NewState()
	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			return nil, nil, fmt.Errorf("replay fixture op %s %s: %w", op.Type, op.TargetID, err)
		}
	}
	return state, state.BuildIndex(), nil
}

func loadEmbeddedOps() ([]ops.Op, error) {
	data, err := fixtureGraph.ReadFile(fixtureOpsName)
	if err != nil {
		return nil, fmt.Errorf("open embedded fixture ops %s: %w", fixtureOpsName, err)
	}
	return parseFixtureOps(fixtureOpsName, data)
}

func parseFixtureOps(name string, data []byte) ([]ops.Op, error) {
	var all []ops.Op
	for lineNo, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		op, err := ops.ParseLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse fixture ops %s:%d: %w", name, lineNo+1, err)
		}
		all = append(all, op)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("fixture ops %s is empty", name)
	}
	return all, nil
}

func measureReview(state *materialize.State) ([]byte, error) {
	issue, ok := state.Issues[FixtureShowIssue]
	if !ok || issue == nil {
		return nil, fmt.Errorf("fixture issue %s not found", FixtureShowIssue)
	}
	diff, err := fixtureGraph.ReadFile(fixtureDiffName)
	if err != nil {
		return nil, fmt.Errorf("read embedded fixture delivery diff: %w", err)
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
