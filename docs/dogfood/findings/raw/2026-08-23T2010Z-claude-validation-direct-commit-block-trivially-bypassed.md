---
date: 2026-08-23
agent: claude
area: validation
task: Determinism architecture review — verifying which enforcement seams actually bind
tags: [harness-hook, enforcement, gate-bypass, commit-discipline, seam-quality]
---

# The harness hook's direct-commit block matches only a bare `git` and is trivially bypassed

## User Goal

While reviewing whether armature's deterministic seams can enforce guardrails without
rebuilding armature as a driver, I needed to establish what the harness hook actually
blocks today — as opposed to what the docs and skills claim it blocks. An evidence agent
had asserted "git commit blocking" as a shipped capability; I went to source to confirm it
rather than take the assertion.

## Observed

The capability is real. `internal/harnesshook/evaluator.go:44` calls
`isDirectCommitCommand(event.Command)` on PreToolUse and returns
`Decision{Action: DecisionBlock}` with the message "Armature owns commits during harness
execution; do not run git commit directly". It correctly skips git global options,
including the flags that consume their next token (`-C`, `-c`, `-f`), so
`git -C /some/path commit` is caught.

But the matcher's first test is:

```go
fields := strings.Fields(command)
if len(fields) == 0 || fields[0] != "git" {
    return false
}
```

The command is only inspected when its **first whitespace-delimited field is the literal
string `git`**. Every one of these therefore passes straight through the block:

- `sh -c 'git commit -m "..."'` / `bash -c '...'` — first field is `sh`/`bash`
- `env git commit` — first field is `env`
- `/usr/bin/git commit` — first field is an absolute path, not `git`
- `cd sub && git commit` — first field is `cd`

The last two are not adversarial constructions. An absolute path is a normal thing for a
script to emit, and `cd … && git commit` is an extremely common shell idiom that an agent
would produce without any intent to evade.

## Impact

This is the failure shape the `unknown-recorded-as-answered` theme describes, one layer
down: the seam reports as enforcing, and the enforcement is real for the exact literal
form it was written against, but "not matched" is indistinguishable from "checked and
allowed". Nothing is logged when the guard declines to match, so a bypass leaves no trace
that the guard was even consulted.

It also directly weakens a claim I was about to rely on in an architecture review — that
command-pattern denial at PreToolUse is a hard seam available for extension to state
transition verbs (e.g. denying `arm transition --to merged`). The mechanism is genuinely
shipped and genuinely useful, but its current matching is strict-literal, so extending it
as-is would inherit the same porousness. A rule that binds only the spelling an obedient
agent would have used anyway is close to no rule at all.

Worth stating plainly: this does not make the hook worthless. Per ADR 0007 and
`docs/harness-hook.md` the hook is deliberately "a best-effort guardrail, not a sandbox",
and defence-in-depth behind a verb-level refusal is the documented posture. The finding is
that the *breadth* of this particular matcher is narrower than its message implies.

## Evidence

`internal/harnesshook/evaluator.go`, `evaluatePreToolUse` and `isDirectCommitCommand`:

```go
func (e *DefaultEvaluator) evaluatePreToolUse(event Event) Decision {
	e.lastScopeViolations = nil
	if isDirectCommitCommand(event.Command) {
		return Decision{
			Action:  DecisionBlock,
			Message: "Armature owns commits during harness execution; do not run git commit directly",
		}
	}
	...
}

func isDirectCommitCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "git" {
		return false
	}
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
```

Hook registration confirming the Bash matcher is live —
`internal/harnesshook/platform_claude.go:103`:

```go
"matcher": "Edit|Write|MultiEdit|Bash",
```

Cross-reference: the `worker-worktree-bypass` theme records commits landing in the wrong
checkout across at least five stories; this matcher is one of the guards that would have
been expected to notice, and would not have if the commit was issued through any of the
forms above.

## Suggested Follow-Up

Two options, in preference order:

1. **Normalise before matching.** Resolve the leading token (strip directory components so
   `/usr/bin/git` reads as `git`), and recurse into the payload of a shell wrapper
   (`sh -c`, `bash -c`, `env`) and across `&&`/`;`/`|` separators, matching each segment.
   Then apply the existing well-tested flag-skipping logic per segment.
2. **Log the non-match.** Whichever breadth is chosen, emit a pass-through log entry when a
   Bash event contains the substring `commit` but does not match, so that "the guard did not
   fire" becomes visible at merge time rather than silent. This is the `not-checked` third
   value applied at the seam level.

If the direct-commit block is later extended to state-transition verbs (`arm transition
--to merged`, `arm merged`), do that work on top of the normalised matcher rather than the
current literal one — otherwise the new rule ships with a known bypass on day one.
