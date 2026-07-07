# Admit harness-recorded execution evidence into semantic review, upgrade-only

Status: accepted — amends ADR-0005 and the 2026-06-27 semantic conformance review design (Non-Goals: "do not collect general tool activity… or harness activity logs"; Architecture: "hooks… neither collect activity for nor initiate conformance review")

Behavioral criteria ("endpoint returns 401") often cannot be established from the delivery diff, forcing the Reviewer to `indeterminate` even when the behavior was demonstrably exercised. We now let harness hooks capture **Execution Evidence** — harness-recorded `(command, output)` pairs from the bound issue's session — and admit it into semantic review as a third, explicitly weaker evidence class. Hooks still never initiate review, never block on review results, and worker prose remains inadmissible; what changed is that *platform-recorded* execution facts are no longer conflated with the untrusted worker-authored activity ADR-0005 meant to exclude.

## The trust model

Execution evidence is harness-recorded, not model-authored — stronger than the worker's Outcome prose, weaker than the fingerprinted diff (the worker chooses what to run, the environment at execution time is unverifiable, and command output can be staged). Three rules keep it from corrupting the assessment:

1. **Upgrade-only.** It can lift `indeterminate` → `satisfied`/`partially_satisfied` on behavioral criteria. It can never substitute for diff citations on implementation criteria, and never suppress a `not_satisfied` the diff supports.
2. **Complete capture, no selection.** The hook appends every qualifying execution (Bash PostToolUse for the bound issue) to a worktree-local **Activity Log**; failure-then-success sequences stay visible instead of being curated by the worker. Per-entry output truncation (content-neutral, with full-output hash) is allowed; command filtering is not — filtering is selection bias through the back door.
3. **HEAD-anchored.** Each entry records the worktree HEAD at execution time; only entries at the delivery commit carry full weight.

## Mechanics

- The Activity Log lives beside the hook decision log: worktree-local, ephemeral, never in the ops log, never committed. Worktree teardown must follow `arm review record` — previously a happenstance of ordering, now a hard constraint.
- The Review Bundle gains an **optional** `activity` section: log digest, entry count, HEAD-anchor summary, path. Absence degrades gracefully — no hooks means no section means today's behavior (more `indeterminate`, never wrongness). The digest enters the attestation.
- A cheap fresh-context subagent produces a schema-validated **Activity Index** at prepare time — a finding aid routing the Reviewer to raw entries. The index is never citable: `arm review record` accepts activity citations only against raw log entry IDs, so a reviewer that read only the summary produces no valid activity citations and the criterion conservatively stays `indeterminate`. An index/log digest mismatch is detectable at record time.

## Disclosure posture

Capture is **default-on** wherever hooks are installed (calibration data must exist before this class could ever graduate beyond advisory), with a repo-level kill-switch. The disclosure path — raw output → reviewer → detailed report → PR surface — is cut at the citation boundary: published reports carry entry ID, command line, and exit status, but not raw output excerpts. A dedicated sensitive-environments document must state all of this loudly.

## Rejected alternatives

- **Activity data for coverage/provenance corroboration** (verifying `changed_files`, detecting out-of-band writes) — rejected as review evidence: inside the worktree the fingerprinted diff is already authoritative; the residue is integrity telemetry, not assessment input.
- **Contradiction-capable evidence** (recorded failing runs dragging criteria to `not_satisfied`) — rejected: the gaming/environment caveats that make this class weak cut both ways, and downgrade power would make staged failures an attack on legitimate deliveries.
- **Citable summaries** — rejected: a cheap model's conclusions wearing deterministic-evidence clothing is worse than the worker prose the design already distrusts.
- **Opt-in capture** — rejected: starves exactly the calibration evidence the phase-one design requires before any enforcement discussion.
