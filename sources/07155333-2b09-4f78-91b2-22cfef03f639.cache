---
area: tooling
writer: 5207ee28
date: 2026-08-14T23:52Z
story: LNGHZN-S10
---

# Scope-overlap detection matches on directory prefix, so unrelated files in the same tree collide

## What the agent-user was trying to do

Claim `LNGHZN-S10-T2` (scope: `Makefile`, `scripts/check-fast.sh`,
`scripts/test-check-fast.sh`, `docs/agents/quality-gates.md`) while
`LNGHZN-S7-T1` was in flight under story `LNGHZN-S7` (scope: `internal/config/*`,
`cmd/armature/{claim,render_context,claim_test}.go`, `internal/doctor/*`,
`docs/use-cases.md`, `docs/configuration.md`).

The two scopes share **no file**. Not one.

## What happened

```text
Error: cannot claim LNGHZN-S10-T2: scope overlap with LNGHZN-S7
       (Make configuration honest — wire or delete every dead knob (LH D1))
       — use --force to override
```

The cause is in `internal/claim/overlap.go`:

```go
func globOverlaps(a, b string) bool {
	if matched, _ := filepath.Match(a, b); matched { return true }
	if matched, _ := filepath.Match(b, a); matched { return true }
	dirA := extractDir(a)
	dirB := extractDir(b)
	if dirA == "" || dirB == "" { return false }
	return dirA == dirB || hasPrefix(dirA, dirB+"/") || hasPrefix(dirB, dirA+"/")
}
```

When both glob directions fail, it falls back to comparing the **containing
directories**. Here `extractDir("docs/agents/quality-gates.md")` is
`docs/agents`, `extractDir("docs/use-cases.md")` is `docs`, and
`hasPrefix("docs/agents", "docs/")` is true — so two entirely different files
are reported as an overlap.

The rule as written means: *any file anywhere under `docs/**` conflicts with any
file directly in `docs/`*. The same holds for every nested tree in the repo.

## How it changed behavior, confidence, or time spent

Two distinct harms.

**It blocks legitimate parallel work.** `T2` could not be claimed at all while an
unrelated story touched an unrelated file in a parent directory. The only
offered remedy is `--force`, which is the override reserved for *reviewed, real*
conflicts — so the false positive spends the exact escape hatch that should stay
meaningful, and trains coordinators to reach for `--force` reflexively.

**It is the source of the `arm validate` warning wall.** The ~200 scope-overlap
warnings currently emitted are overwhelmingly this false positive. The warning
text itself is the tell — it prints the two paths and they are visibly different
files:

```text
WARNING: scope overlap: LNGHZN-S10-T5 and TOPTIER-S8-T1 both modify
    docs/design/gate-efficiency.md <-> docs/why-armature.md (new)
WARNING: scope overlap: TOPTIER-S15-T2 and TOPTIER-S7-T2 both modify README.md
```

Only the second form (identical path, no `<->`) is a true overlap.

**This collides head-on with this story's own D7.** `LNGHZN-S10-T4` makes
`arm validate` strict by default — warnings become errors — and its rollout
requires "burning down all existing warnings". That burn-down is impossible
while most warnings are false positives with no legitimate fix: there is nothing
to correct in a plan where two tasks own two different files that happen to
share an ancestor directory. D7 also says "rules that fire on intentional states
get fixed or deleted, not waived", which is precisely this rule's situation.
**T4 should be treated as blocked on fixing this heuristic**, or strict-by-default
will make the DAG unclaimable.

## What would have helped

The directory fallback appears to be reaching for "these tasks are working in
the same area, so warn". That intent is defensible as an *advisory* signal but
is wrong as a *blocking* equality test, and wrong at directory granularity.

- **Match on files, not directories.** Two scopes overlap when a concrete path
  or glob actually matches the same file. Drop the `extractDir` fallback from
  the blocking path entirely.
- If a same-area heuristic is still wanted, make it a separate, clearly-labelled
  advisory that never blocks a claim and never fails strict validation — and
  make it match on a meaningful unit (same package, same censused surface), not
  on any shared ancestor directory.
- Directory-level scope entries should be expressed as explicit globs
  (`docs/agents/**`) and matched as globs, so a task that genuinely owns a whole
  tree still conflicts correctly without inferring ownership from a filename's
  parent.
- Whatever the fix, `arm validate`'s warning output should distinguish
  "same file" from "same area" so the two are never confused again.
