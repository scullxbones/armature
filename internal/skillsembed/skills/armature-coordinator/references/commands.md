# Command Reference

## Querying JSON Output

Structured `arm` output (including `arm list`) is one JSON envelope object
per command: `{count, issues[], help[]}`. Parse and count entries in
`.issues`. Do not `grep` / `grep -c` the envelope as if each issue were its
own line — the whole envelope is one line, so a line match is 1 whenever
any issue matches.

```bash
# Extract a single field from each issue
arm list --parent STORY-ID | jq -r '.issues[].status'

# Filter issues where a field matches a value
arm list --parent STORY-ID | jq '.issues[] | select(.status=="done")'

# Count matches (use jq length, not grep -c)
arm list --parent STORY-ID | jq '[.issues[] | select(.status=="done")] | length'

# Extract IDs of all blocked tasks
arm list --status blocked | jq -r '.issues[].id'

# Show title alongside status for a quick overview
arm list --parent STORY-ID | jq -r '.issues[] | [.id, .title, .status] | @tsv'
```

`.count` is the length of `.issues` after CLI filters (`--parent`,
`--status`, …). Use `jq` select for subsets of that payload.

---

## Full Command Reference

```bash
# Surveying work
arm ready                              # unblocked, unclaimed tasks
arm ready --assigned-to WORKER-ID      # tasks pre-assigned to a specific worker
arm list --status blocked              # diagnose blockers
arm list --status in-progress          # in-flight claims
arm list --parent STORY-ID             # all tasks in a story

# Assignment (pre-wire before dispatching)
arm assign --issue ID --worker WORKER-ID   # pre-assign (does not claim)
arm unassign --issue ID                     # release assignment

# Claiming and context
arm claim --issue ID --worktree --ttl 120  # claim (marks in-progress, sets TTL, creates worktree at .worktrees/<issue-id>)
arm render-context --issue ID --budget 4000  # assemble full task context

# Validation and story close
arm validate                    # citation coverage + source UUID integrity
arm validate --ci               # exit non-zero on errors (for CI use)
arm transition ID --to done --outcome "..."   # close task or story
arm doctor                      # repo health check
arm doctor --strict             # warnings as errors

# Monitoring
arm workers                     # worker activity status

# Scope maintenance (after file renames or deletions)
arm scope-rename OLD-PATH NEW-PATH  # rewrite path/prefix across all issue scopes
arm scope-delete PATH           # remove exact file path from all issue scopes
```

**Valid transition targets:** `in-progress`, `done`, `cancelled`, `blocked`
