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

Preconditions: launch succeeded; target is bootstrapped (helper bootstraps if `.armature/` is missing). Do not intend to rotate identity.

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh drive worker-init
```

Raw equivalent:

```bash
"$ARM" --repo "$TARGET" worker-init --check
git -C "$TARGET" config --get armature.worker-id
"$ARM" --repo "$TARGET" worker-init --check   # second read: same uuid
```

Proof: both `--check` lines match; git config value equals the printed uuid. Evidence: `evidence/<run-id>/drive/01-check-before/` and `03-check-after/`.

## Gotchas

- Unflagged `arm worker-init` **always generates a new UUID** and overwrites `armature.worker-id`. Never "just run worker-init again" to confirm.
- The documented session idiom is `--check || worker-init`, not bare `worker-init`.
- Output is not agent JSON. Parse the `Worker ID: ` prefix or read git config.
- If you overwrite the id after writing ops, later materialize/D7 can see worker-id mismatches against existing log filenames.
