---
area: tooling
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
date: 2026-08-14T20:21Z
story: LNGHZN-S9
---

# Safety hook mistakes `arm unlink` for POSIX file deletion

## What the agent-user was trying to do

Read `arm unlink --help` while diagnosing whether an erroneous task dependency
could be removed through the supported Armature command.

## What happened

The PreToolUse safety hook blocked the entire read-only compound command:

```text
Command blocked by PreToolUse hook: BLOCKED by dcg
Reason: unlink is destructive (POSIX equivalent of rm on a single file)
```

The executable was `arm`; `unlink` was an Armature subcommand that appends a DAG
relationship-removal op. `arm help unlink` returned the intended Cobra help and
confirmed that meaning.

## How it changed behavior, confidence, or time spent

The false positive prevented even help discovery and made an application-level
workflow verb look like an attempted filesystem deletion. The coordinator had to
rephrase a harmless query and must now anticipate whether executing the legitimate
subcommand will also be blocked.

## Evidence

- Blocked form: `arm unlink --help`.
- Working equivalent: `arm help unlink`.
- Returned description: `Remove a dependency relationship between two issues.`

## What would have helped

The hook should classify the executable token, not every argument token, when
applying shell-command destruction rules. `unlink` should be treated as POSIX
deletion only when it is the executable being invoked.
