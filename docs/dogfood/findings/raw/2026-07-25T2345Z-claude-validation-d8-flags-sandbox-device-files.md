# Finding: arm doctor D8 flagged sandbox bind-mount device files as out-of-scope artifacts

**Writer:** claude
**Area:** validation

## What I was trying to do
Run `arm doctor` after merging LNGHZN-S4-T2 to confirm repo health before
proceeding to the next task in the wave, per the coordinator skill's DAG
hygiene mandate.

## What happened
`arm doctor` reported a D8 error listing ~16 "out-of-scope artifacts" for
LNGHZN-S4-T2: `.bash_profile`, `.bashrc`, `.claude/agents`,
`.claude/commands`, `.gitconfig`, `.mcp.json`, `.zshrc`, etc. These are not
real repository content — `ls -la` shows them as character devices
(`crw-rw-rw- 1 nobody nogroup 1, 3 ... .bash_profile`, major/minor 1,3 =
`/dev/null`), i.e. sandbox session scaffolding bind-mounted into the repo
root by the harness, not files any task wrote. D8 flagged them purely
because they were untracked paths present in the working tree during that
task's activity window; it has no way to distinguish real out-of-scope
writes from environment noise that happens to sit in the repo root.

## How it changed behavior, confidence, or time spent
Cost one diagnostic round-trip (`ls -la`, checking major/minor device
numbers) to confirm these weren't real violations before proceeding. Not
fixed — this is specific to this sandboxed/background-job environment and
out of scope for the LNGHZN-S4 story itself — but worth tracking since a
less-careful coordinator could mistake this for a genuine scope violation
and either wrongly block a merge or wrongly `--force` past a real one.

## Evidence
- `arm doctor` output: `✗ D8: Out-of-scope artifacts detected for active or
  recently-completed tasks\n    - LNGHZN-S4-T2: .bash_profile\n    -
  LNGHZN-S4-T2: .claude/agents\n    ...` (16 entries)
- `ls -la .bash_profile .bashrc .gitconfig .mcp.json`:
  `crw-rw-rw- 1 nobody nogroup 1, 3 Jul 24 23:53 .bash_profile` (repeated for
  each — all character devices to `/dev/null`, not regular files).
