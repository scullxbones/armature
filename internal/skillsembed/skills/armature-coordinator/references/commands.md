# Command Reference

Coordinator JSON extraction only. Full CLI help is `docs/commands.md`. Do not keep a second command cheat-sheet in this skill tree.

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
