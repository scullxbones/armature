package workerruntime

// EventTier identifies the persistence sink for runtime events.
type EventTier string

const (
	EventTierTrace    EventTier = "trace"
	EventTierSnapshot EventTier = "snapshot"
	EventTierDurable  EventTier = "durable"
)

// RuntimeEvent is a normalized event emitted by the worker runtime.
type RuntimeEvent struct {
	Type           string
	SharedDecision bool
}

const (
	EventPollStarted        = "poll_started"
	EventNoReadyWork        = "no_ready_work"
	EventClaimLost          = "claim_lost"
	EventExecutionCompleted = "execution_completed"
	EventCooldownScheduled  = "cooldown_scheduled"
	EventPauseCheckpoint    = "pause_checkpoint"
	EventStopRequested      = "stop_requested"
	EventHumanEscalation    = "human_escalation"
)

// DurableAdmission classifies runtime events into persistence tiers.
func DurableAdmission(ev RuntimeEvent) EventTier {
	switch ev.Type {
	case EventPollStarted, EventNoReadyWork, EventClaimLost, EventExecutionCompleted:
		return EventTierTrace
	case EventCooldownScheduled, EventPauseCheckpoint, EventStopRequested:
		return EventTierSnapshot
	case EventHumanEscalation:
		if ev.SharedDecision {
			return EventTierDurable
		}
		return EventTierSnapshot
	default:
		if ev.SharedDecision {
			return EventTierDurable
		}
		return EventTierSnapshot
	}
}
