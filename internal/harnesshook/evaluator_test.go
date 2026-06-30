package harnesshook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatorBlocksOutOfScopeEdit(t *testing.T) {
	t.Parallel()
	service := harnesspolicy.NewVerificationService()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
		VerificationService: &service,
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:  EventPreToolUse,
		Tool:  "Edit",
		Paths: []string{"cmd/armature/main.go"},
	})

	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Action)
	assert.Contains(t, decision.Message, "outside task scope")
}

func TestEvaluatorAllowsInScopeEdit(t *testing.T) {
	t.Parallel()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:  EventPreToolUse,
		Tool:  "Edit",
		Paths: []string{"internal/orchestrate/engine.go"},
	})

	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Action)
}

func TestEvaluatorBlocksGitCommit(t *testing.T) {
	t.Parallel()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{
		Kind:    EventPreToolUse,
		Tool:    "Bash",
		Command: "git commit -m 'agent commit'",
	})

	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Action)
	assert.Contains(t, decision.Message, "Armature owns commits")
}

func TestEvaluatorBlocksGitCommitWithGlobalOptions(t *testing.T) {
	t.Parallel()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy: harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
	})

	for _, cmd := range []string{
		"git -C /repo commit -m 'msg'",
		"git -c user.name=x commit -m 'msg'",
		"git --no-pager commit -m 'msg'",
	} {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			decision, err := evaluator.Evaluate(context.Background(), Event{
				Kind:    EventPreToolUse,
				Tool:    "Bash",
				Command: cmd,
			})
			require.NoError(t, err)
			require.Equal(t, DecisionBlock, decision.Action, "should block: %s", cmd)
			assert.Contains(t, decision.Message, "Armature owns commits")
		})
	}
}

func TestEvaluatorRunsStopVerification(t *testing.T) {
	t.Parallel()
	service := harnesspolicy.NewVerificationService()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         harnesspolicy.NewScopePolicy([]string{"internal/orchestrate/"}),
		VerificationService: &service,
		VerificationInput: harnesspolicy.VerificationRequest{
			Acceptance: json.RawMessage(`["go test ./... passes"]`),
			Citations: []harnesspolicy.CitationCheck{
				{SourceEntryID: "SRC-1", Accepted: true},
			},
		},
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{Kind: EventStop})

	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Action)
	assert.Contains(t, decision.Message, "verification passed")
}

func TestEvaluatorBlocksStopWhenVerificationFails(t *testing.T) {
	t.Parallel()
	service := harnesspolicy.NewVerificationService()
	evaluator := NewEvaluator(EvaluatorConfig{
		VerificationService: &service,
		VerificationInput: harnesspolicy.VerificationRequest{
			Acceptance: json.RawMessage(`["human review only"]`),
		},
	})

	decision, err := evaluator.Evaluate(context.Background(), Event{Kind: EventStop})

	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Action)
	assert.Contains(t, decision.Message, "unverifiable")
}
