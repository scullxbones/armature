# Dogfood Findings

Armature is used to build Armature — every task is a live dogfood run. Capture friction immediately, then return to task. Don't turn findings into implementation work unless asked.

```
Skill("capturing-dogfood-findings")
```

Invoke when:

- an `arm` command fails, returns confusing output, or behaves differently than the docs describe
- a skill does not fire when expected, or fires with wrong content
- a workflow step requires knowledge not in AGENTS.md or the relevant skill
- `make check`, `arm validate`, or `arm doctor` behaves unexpectedly
- a doc or error message is misleading or missing

Write one raw finding under `docs/dogfood/findings/raw/`, from the agent-user perspective.

- **Writer identity:** `arm worker-init --check` (UUID) + `ARM_LOG_SLOT` if set
- **Area:** `bootstrap` | `hooks` | `skills` | `commands` | `workflow` | `validation` | `coordination` | `tooling` | `documentation` | `other`
