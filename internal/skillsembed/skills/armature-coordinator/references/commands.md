# Command Reference

## Querying JSON Output

Most `arm` commands emit newline-delimited JSON. Use `grep` for quick
field extraction without requiring `jq`:

```bash
# Extract a single field from each object
arm list --parent STORY-ID | grep -o '"status":"[^"]*"'

# Filter objects where a field matches a value
arm list --parent STORY-ID | grep '"status":"done"'

# Count matches
arm list --parent STORY-ID | grep -c '"status":"done"'

# Extract IDs of all blocked tasks
arm list --status blocked | grep -o '"id":"[^"]*"'

# Show title alongside status for a quick overview
arm list --parent STORY-ID | grep -o '"id":"[^"]*"\|"title":"[^"]*"\|"status":"[^"]*"'
```

These patterns work in any shell without additional tooling. If `jq` is
available you can use it for more complex queries, but `grep` is sufficient
for the common coordinator workflow.

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
arm claim --issue ID [--ttl 120]            # claim (marks in-progress, sets TTL)
arm render-context --issue ID [--budget 4000]  # assemble full task context

# Validation and story close
arm validate                    # citation coverage + source UUID integrity
arm validate --ci               # exit non-zero on errors (for CI use)
arm transition ID --to done --outcome "..."   # close task or story
arm doctor                      # repo health check
arm doctor --strict             # warnings as errors

# Monitoring
arm workers                     # worker activity status

# Scope maintenance (after file renames or deletions)
arm scope-rename <old> <new>    # rewrite path/prefix across all issue scopes
arm scope-delete <path>         # remove exact file path from all issue scopes
```

**Valid transition targets:** `in-progress`, `done`, `cancelled`, `blocked`
