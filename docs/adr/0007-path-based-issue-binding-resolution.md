# Path-based issue binding resolution in the harness hook

Status: accepted

## Principles touched

I4

Harness workers are dispatched as subagents *inside* the coordinator's harness session, not as separate harness processes. Subagents share the session's hook configuration, environment, and hook working directory — so a hook that resolves the issue binding from its own process cwd attributes every worker's activity to the coordinator's binding (usually none, yielding silent pass-through with no scope enforcement). To preserve the invariant that each agent operates under exactly one Issue Binding, `arm harness-hook` resolves the binding from the **event itself**, most specific first:

1. `tool_input.file_path` — walk up from the target file to the containing worktree's git dir and its `armature-issue-id` file
2. Event payload `cwd` — for harnesses that report per-agent working directories
3. Hook process cwd — the session's own binding (current behavior; covers Bash and Stop events, which carry no file path)
4. `ARMATURE_ISSUE_ID` environment variable — last-resort fallback; launch-time-only fidelity, kept for harnesses without worktree support

Binding identity follows the artifact being touched, not the process touching it.

## Considered options

- **Per-event `cwd` resolution alone** — rejected: Claude Code reports the *session's* cwd on hook events regardless of which subagent fired them, so worker activity in other worktrees would still misresolve.
- **Separate harness process per worker** (launch a real CLI process from inside each worktree) — rejected for now: it makes the invariant structural, but abandons Agent-tool dispatch, result plumbing, and permission inheritance (worktree-isolated subagents already lose Bash permissions), and forces a coordinator rewrite. May be revisited if harnesses grow first-class per-agent hook contexts.
- **Path-based resolution (chosen)** — enforces the invariant with no dispatch-model change; a subagent editing worktree X is evaluated under X's issue policy regardless of which session hosts it.

## Consequences

- Binding resolution must happen **after** the event payload is decoded (it needs `tool_input`), reversing the previous read-binding-then-stdin order.
- Bash and Stop events resolve at the session level (steps 3–4). Stop verification therefore runs against the session's own binding only — a coordinator session is never blocked by its workers' acceptance criteria. Out-of-scope *shell* mutations remain uncaught, as today; Edit/Write scope enforcement is the primary control.
- A file write that resolves to **no** binding is logged as a `violation:` entry in the worktree-local `armature-hook.log` (decisions are never written to the ops log); `arm merged --issue` fails on violations unless forced, while `pass-through:` entries remain warnings. Worktree dispatch is mandatory for workers — without a bound worktree there is nothing for resolution to find.
- All resolution failures are **fail-open with loud stderr messaging**, including snapshot-load and event-decode errors (previously accidentally fail-closed via nonzero exit). Enforcement gaps are surfaced by the violation gate, not by freezing tool use.
