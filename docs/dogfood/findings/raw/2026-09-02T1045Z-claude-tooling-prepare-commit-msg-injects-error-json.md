---
date: 2026-09-02
agent: claude
writer: claude
area: tooling
task: PR #127 review remediation
tags: [git-hooks, prepare-commit-msg, arm-show, stdout-stderr, active-claim]
---

# `prepare-commit-msg` prepends `arm`'s error JSON to the commit subject when there is no active claim

## User Goal

Commit a fix on `feat/verify-armature` while addressing PR review
findings. No armature issue was claimed — the work was PR remediation on
a branch, not a claimed task.

## Observed

`git commit` succeeded, but the resulting subject line was:

```
{"error":{"code":"GENERAL-1","cause":"issue \"active-claim\" not found","next_actions":[],"exit_code":1}}: fix(verify-armature): drop python dependency and tighten drive assertions
```

The hook is doing this:

```sh
claim_id=$(arm show active-claim --field id 2>/dev/null)
if [ -n "$claim_id" ]; then
  original_msg=$(cat "$commit_msg_file")
  echo "$claim_id: $original_msg" > "$commit_msg_file"
fi
```

`arm show active-claim --field id` fails (there is no issue literally named
`active-claim`) and writes its agent-format error envelope to **stdout**.
So `2>/dev/null` suppresses nothing, `$claim_id` is a non-empty JSON blob,
and `[ -n ]` is satisfied. The hook then prefixes the subject with it.

The hook never checks the exit status.

## Impact

The commit landed and was pushed before I noticed. Recovering meant
amending the message and force-pushing over a commit already on the PR
branch — a history rewrite that would have been avoidable.

For the rest of the session (21 more commits) I committed with
`git -c core.hooksPath=<empty dir> commit`, i.e. with **all** armature
hooks disabled, because that was the only way to get a clean subject. So
a cosmetic bug in one hook caused every later commit to bypass the
pre-commit and commit-msg hooks too — including whatever ops-log
protection those provide.

An agent that does not read back its own commit subject will silently
produce commits whose first line is a JSON error object. That breaks the
conventional-commit format the delivery gate looks for, so the damage is
not only cosmetic.

## Evidence

- Hook: `.git/hooks/prepare-commit-msg`, marked `# armature:managed`.
- Bad commit: `fc311f9e` on `feat/verify-armature` (amended to `c62f83f8`
  and force-pushed with `--force-with-lease`).
- `git log -1 --format='%s'` on that commit returned the JSON-prefixed
  subject quoted above.
- Reproduction of the underlying cause: `arm show active-claim --field id`
  prints `{"error":{"code":"GENERAL-1",...}}` on **stdout** and exits 1.
  The same stdout-error pattern shows up in the `arm-verify` drive
  evidence, where `sources sync` errors appear in captured stdout.

## Suggested Follow-Up

Two independent fixes, either of which closes it:

1. Hook: check the exit status, not just emptiness —
   `claim_id=$(arm show active-claim --field id 2>/dev/null) || claim_id=""`,
   and additionally reject a value that is not a bare issue id.
2. `arm`: write error envelopes to **stderr**, not stdout, so `2>/dev/null`
   means what every shell author expects it to mean. This is the more
   general fix — any script capturing `arm ... --field x` has the same trap.
