package orchestrate

import (
	"context"
	"time"
)

// Clock provides deterministic time for orchestration planning.
type Clock interface {
	NowUnix() int64
}

type systemClock struct{}

func (systemClock) NowUnix() int64 {
	return time.Now().Unix()
}

// Runner is the service boundary used by command code.
type Runner interface {
	Run(ctx context.Context, input RunInput) (OrchestrateState, error)
}
