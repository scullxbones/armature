package harnesshook

import (
	"context"
	"strings"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

// EvaluatorConfig holds the policy inputs needed to construct a DefaultEvaluator.
type EvaluatorConfig struct {
	ScopePolicy         harnesspolicy.ScopePolicy
	VerificationService *harnesspolicy.VerificationService
	VerificationInput   harnesspolicy.VerificationRequest
}

// DefaultEvaluator is the standard policy evaluator for harness hook events.
type DefaultEvaluator struct {
	cfg EvaluatorConfig
}

// NewEvaluator constructs a DefaultEvaluator from the provided config.
func NewEvaluator(cfg EvaluatorConfig) *DefaultEvaluator {
	return &DefaultEvaluator{cfg: cfg}
}

// Evaluate applies scope and verification policy to the event and returns a Decision.
func (e *DefaultEvaluator) Evaluate(_ context.Context, event Event) (Decision, error) {
	switch event.Kind {
	case EventPreToolUse:
		return e.evaluatePreToolUse(event), nil
	case EventStop:
		return e.evaluateStop(), nil
	default:
		return Decision{Action: DecisionNone, Message: "event ignored"}, nil
	}
}

func (e *DefaultEvaluator) evaluatePreToolUse(event Event) Decision {
	if isDirectCommitCommand(event.Command) {
		return Decision{
			Action:  DecisionBlock,
			Message: "Armature owns commits during harness execution; do not run git commit directly",
		}
	}
	if len(event.Paths) == 0 {
		return Decision{Action: DecisionAllow, Message: "no path policy applies"}
	}
	result := e.cfg.ScopePolicy.CheckPaths(event.Paths)
	if !result.Allowed {
		return Decision{Action: DecisionBlock, Message: result.Message()}
	}
	return Decision{Action: DecisionAllow, Message: "path is within task scope"}
}

func (e *DefaultEvaluator) evaluateStop() Decision {
	if e.cfg.VerificationService == nil {
		return Decision{Action: DecisionAllow, Message: "no verification service configured"}
	}
	results := e.cfg.VerificationService.Run(e.cfg.VerificationInput)
	for _, result := range results {
		if !result.Passed {
			return Decision{Action: DecisionBlock, Message: result.Message}
		}
	}
	return Decision{Action: DecisionAllow, Message: "verification passed"}
}

func isDirectCommitCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "git" {
		return false
	}
	// Skip git global options (flags and their arguments) before the subcommand.
	// Flags that consume the next token as their argument:
	flagsTakingArg := map[string]bool{"-C": true, "-c": true, "-f": true}
	i := 1
	for i < len(fields) {
		f := fields[i]
		if !strings.HasPrefix(f, "-") {
			return f == "commit"
		}
		if flagsTakingArg[f] {
			i += 2
		} else {
			i++
		}
	}
	return false
}
