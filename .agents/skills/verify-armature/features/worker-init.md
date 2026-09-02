# Register worker identity

`arm worker-init` stores a UUID in local git config `armature.worker-id`. Every later write (create, claim, ops log name) is attributed to that id. `--check` is the read-only form used at the start of a session.

## Sub-features

- `--check`: print `Worker ID: <uuid>` or fail if unset
- Unflagged run: generate a new UUID and write git config
- Bootstrap already inits the id when missing, so a fresh bootstrapped clone usually already has one
- `ARM_LOG_SLOT` suffixes the **log filename** (`<uuid>~<slot>.log`) without changing the configured UUID

## How to get to it (user POV)

Once per clone, after bootstrap:

```bash
arm --repo "$TARGET" worker-init --check || arm --repo "$TARGET" worker-init
```

Stdout is always the line `Worker ID: <uuid>` (not JSON, even with `--format agent`).

## Driving it with arm-verify.sh

Preconditions: launch succeeded; target is bootstrapped (helper bootstraps if `.armature/` is missing). The drive rotates the identity **inside the temp repo only** — it clears `armature.worker-id` first, because bootstrap already wrote one and driving `--check` alone would never exercise identity creation.

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive worker-init
```

Raw equivalent:

```bash
git -C "$TARGET" config --unset armature.worker-id     # temp repo only
"$ARM" --repo "$TARGET" worker-init --check            # must fail when unset
"$ARM" --repo "$TARGET" worker-init                    # creates the uuid
"$ARM" --repo "$TARGET" worker-init --check
git -C "$TARGET" config --get armature.worker-id
"$ARM" --repo "$TARGET" worker-init --check            # second read: same uuid
```

Proof: `--check` exits nonzero while unset; unflagged `worker-init` writes the id and echoes it; both later `--check` lines match each other and equal `Worker ID: <git config value>`. Evidence: `evidence/<run-id>/drive/02-check-unset/` through `06-check-again/`.

## Gotchas

- Unflagged `arm worker-init` **always generates a new UUID** and overwrites `armature.worker-id`. Never "just run worker-init again" to confirm.
- The documented session idiom is `--check || worker-init`, not bare `worker-init`.
- Output is not agent JSON. Parse the `Worker ID: ` prefix or read git config.
- If you overwrite the id after writing ops, later materialize/D7 can see worker-id mismatches against existing log filenames.
