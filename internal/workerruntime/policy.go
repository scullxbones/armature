package workerruntime

import "time"

// Policy holds the conservative built-in runtime policy defaults.
type Policy struct {
	Retry struct {
		MaxVerificationFailures int
		BudgetPerClass          map[string]int
	}
	Cooldown struct {
		NoReadyWorkDelay time.Duration
		WorkerLocalDelay time.Duration
	}
	Subworkflows struct {
		ExecutionLaneRouting struct {
			Workflow              string
			ExceptionAgentEnabled bool
		}
		RuntimeRecovery struct {
			Workflow              string
			ExceptionAgentEnabled bool
		}
		TaskContractReview struct {
			Workflow              string
			ExceptionAgentEnabled bool
		}
		ScopeAndDependencyResolution struct {
			Workflow              string
			ExceptionAgentEnabled bool
		}
		ProvenanceReview struct {
			Workflow              string
			ExceptionAgentEnabled bool
		}
	}
	Worker struct {
		Model   string
		Timeout int
	}
	Quota struct {
		MaxTasksPerRun int
	}
	Escalation struct {
		Enabled bool
	}
}

// DefaultPolicy returns conservative v1 runtime policy defaults.
// Exception-agent execution is disabled by default.
func DefaultPolicy() Policy {
	var p Policy

	p.Retry.MaxVerificationFailures = 1
	p.Retry.BudgetPerClass = map[string]int{
		"timeout":   2,
		"transient": 3,
	}

	p.Cooldown.NoReadyWorkDelay = 30 * time.Second
	p.Cooldown.WorkerLocalDelay = 5 * time.Second

	p.Subworkflows.ExecutionLaneRouting.Workflow = "execution_lane_routing"
	p.Subworkflows.ExecutionLaneRouting.ExceptionAgentEnabled = false

	p.Subworkflows.RuntimeRecovery.Workflow = "runtime_recovery"
	p.Subworkflows.RuntimeRecovery.ExceptionAgentEnabled = false

	p.Subworkflows.TaskContractReview.Workflow = "task_contract_review"
	p.Subworkflows.TaskContractReview.ExceptionAgentEnabled = false

	p.Subworkflows.ScopeAndDependencyResolution.Workflow = "scope_and_dependency_resolution"
	p.Subworkflows.ScopeAndDependencyResolution.ExceptionAgentEnabled = false

	p.Subworkflows.ProvenanceReview.Workflow = "provenance_review"
	p.Subworkflows.ProvenanceReview.ExceptionAgentEnabled = false

	p.Worker.Model = "claude-sonnet-4-6"
	p.Worker.Timeout = 1800

	p.Quota.MaxTasksPerRun = 0 // 0 = unlimited

	p.Escalation.Enabled = false

	return p
}
