package orchestrate

import (
	"context"
	"fmt"
)

// ServiceConfig contains the imperative ports used by the service boundary.
type ServiceConfig struct {
	Git     GitClient
	OpLog   OpLog
	Harness HarnessAdapter
	Clock   Clock
}

// RunInput carries the dynamic orchestration inputs for one run.
type RunInput struct {
	TaskID       string
	WorkerID     string
	RetryBudget  int
	Scope        []string
	ActiveScopes map[string][]string
	HarnessCfg   HarnessConfig
	Opts         RunOptions
}

// Service is the orchestration application-service boundary used by command code.
type Service struct {
	Config ServiceConfig
}

func NewService(cfg ServiceConfig) *Service {
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}
	return &Service{Config: cfg}
}

func (s *Service) Run(ctx context.Context, input RunInput) (OrchestrateState, error) {
	if s.Config.Harness != nil {
		engine := NewEngine(EngineConfig{
			TaskID:       input.TaskID,
			Git:          s.Config.Git,
			OpLog:        s.Config.OpLog,
			Harness:      s.Config.Harness,
			HarnessCfg:   input.HarnessCfg,
			Scope:        input.Scope,
			ActiveScopes: input.ActiveScopes,
			Opts:         input.Opts,
			RetryBudget:  input.RetryBudget,
			WorkerID:     input.WorkerID,
		})
		return engine.Run(ctx)
	}

	if s.Config.Git == nil || s.Config.OpLog == nil {
		return OrchestrateState{}, fmt.Errorf("service requires git and op-log ports")
	}

	allOps, err := s.Config.OpLog.ReadAll()
	if err != nil {
		return OrchestrateState{}, fmt.Errorf("read op log: %w", err)
	}
	state := DeriveState(allOps, input.TaskID)
	if input.Opts.DryRun {
		return state, nil
	}

	head, err := s.Config.Git.HeadSHA()
	if err != nil {
		return OrchestrateState{}, fmt.Errorf("head sha for dispatch: %w", err)
	}

	next, effects := PlanNextStep(state, DecisionInput{
		TaskID:         input.TaskID,
		WorkerID:       input.WorkerID,
		NowUnix:        s.Config.Clock.NowUnix(),
		RetryBudget:    input.RetryBudget,
		Scope:          input.Scope,
		ActiveScopes:   input.ActiveScopes,
		PreDispatchRef: head,
		WorktreePath:   input.Opts.WorkDir,
	})
	for _, effect := range effects {
		if effect.Kind != EffectAppendDispatchOp {
			continue
		}
		if err := s.Config.OpLog.Append(effect.Op); err != nil {
			return OrchestrateState{}, fmt.Errorf("append dispatch op: %w", err)
		}
	}

	return next, nil
}
