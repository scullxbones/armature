# Command Reference

Coordinator JSON extraction only. Full CLI help is `docs/commands.md`. Do not keep a second command cheat-sheet in this skill tree.

## Querying JSON Output

Most `arm` commands emit newline-delimited JSON. `grep` is enough for the common loop:

```bash
arm list --parent STORY-ID | grep -o '"status":"[^"]*"'
arm list --parent STORY-ID | grep '"status":"done"'
arm list --parent STORY-ID | grep -c '"status":"done"'
arm list --status blocked | grep -o '"id":"[^"]*"'
arm list --parent STORY-ID | grep -o '"id":"[^"]*"\|"title":"[^"]*"\|"status":"[^"]*"'
```

Use `jq` when the query is more than field extraction.
