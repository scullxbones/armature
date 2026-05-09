# Orchestrate Readiness And Executability

## Summary

The brainstorming identified a useful separation between:

- **Definition of Ready (DoR)**: is the task well-formed and governable enough
  to enter the executable pool?
- **Definition of Executability (DoE)**: can this worker class execute the task
  right now in the current environment?

This avoids overloading "readiness" with transient runtime conditions such as
 provider quota exhaustion or harness outages.

## Outcome Classes

The internal workflow model should prefer these outcomes over a generic
 "warning" bucket:

- `pass`
- `informational`
- `policy_evaluable`
- `blocked`

### Meaning

- `pass`: no action needed
- `informational`: audit/ranking metadata only
- `policy_evaluable`: must enter a deterministic or bounded sub-workflow
- `blocked`: task cannot proceed until fixed

## Definition of Ready

DoR should focus on task quality, governance, dependencies, and provenance.

### Typical `blocked` DoR conditions

- task is still `draft`
- imported/inferred node is not confirmed
- missing parent or dependency references
- dependency cycle exists
- missing `definition_of_done`
- missing or empty acceptance criteria
- missing or invalid scope
- missing source linkage or accepted citation
- source IDs do not resolve
- accepted overlap lacks a required rationale

### Typical `policy_evaluable` DoR conditions

- DoD appears vague
- acceptance quality appears weak
- scope may be overbroad
- scope and acceptance may be inconsistent
- task may be too large for one execution cycle
- task may hide multiple unrelated deliverables
- unresolved sibling overlap needs a policy decision
- conflicting decisions may affect the task
- citation may be semantically weak
- task may need a stronger worker/model tier

### Typical `informational` DoR conditions

- new files not marked explicitly, but inferable
- priority missing
- TTL expectation looks suboptimal
- fallback execution path exists but is not preferred

## Definition of Executability

DoE should focus on runtime and worker-class conditions.

### Typical `blocked` DoE conditions

- required blockers are not merged
- repo structural health fails
- verification commands are not configured
- harness adapter is missing and no deterministic fallback exists
- sandbox prerequisites are unavailable and no deterministic fallback exists

### Typical `policy_evaluable` DoE conditions

- preferred model unavailable and fallback choice matters
- harness credits exhausted
- provider outage or auth failure
- verification failures persist
- repeated timeouts occur
- refreshed provenance changes task meaning
- task-quality mismatch emerges only during execution

### Typical `informational` DoE conditions

- preferred model unavailable but safe fallback is already configured
- fallback harness/model path exists

## Practical Intent

DoR should remove tasks that never should have reached workers.

DoE should handle real-time execution conditions without pretending they are
 properties of the task itself.

## Design Principle

Ruthlessly minimize human acknowledgment. If a deterministic rule or a policy
 can make the choice with high confidence, route there before any human review.
