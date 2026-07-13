# ADR: Context Files Intent, Lifecycle, and CLI Semantics

## Status

Accepted

## Context

Armature tasks already carry `scope`, `acceptance`, `definition_of_done`, and optional free-form `context`. The `context_files` field exists to name stable background documents that should be rendered for an agent before it starts work, especially when the implementation scope spans several directories.

The current validator warns when an active issue spans three or more directories without `context_files`. That warning is useful, but dogfooding exposed a mismatch: `create`, `amend`, and `decompose-apply` do not provide a clear operator path for setting or maintaining the field. This makes the validator ask for data the command surface cannot reliably supply.

## Decision

`context_files` is an explicit curation field, not a duplicate of `scope`.

- `scope` answers: which repository paths may the task change?
- `context_files` answers: which stable documents or reference files should be read before changing anything?
- `context` answers: which short snippets or notes should travel with this task?

For active tasks with broad multi-directory scope, `context_files` remains an advisory validation warning until the CLI fully supports the field. It must not become a hard error while operators lack complete create/amend/decompose support.

## Examples

Good uses:

- A task touching `cmd/armature`, `internal/materialize`, and `internal/ops` includes the relevant design doc that constrains the implementation.
- A task implementing source sync behavior includes `docs/design/architecture.md` sections covering source manifests and provider cache semantics.
- A refactor task includes the approved design or ADR that constrains architectural boundaries.

Anti-patterns:

- Copying every scoped source file into `context_files`.
- Adding volatile generated files, runtime state, coverage output, local credentials, or provider config.
- Using `context_files` to expand write permission. Write permission remains governed by `scope`.
- Adding vague catch-all entries such as `docs/` or `**/*.md`.

## Lifecycle

Task creation should set `context_files` when the worker needs durable background context that is not obvious from the changed file list. Decomposition should carry context documents from the parent plan or source citation when those documents constrain the task. Amendment should replace the full list when task scope or intent changes.

When a context file is renamed, the same maintenance path used for scope rename should update `context_files`. When a file is deleted, validation may report a phantom context entry and operators should either restore the document, replace the entry, or remove it.

## Command Surface Requirements

The CLI should expose `context_files` consistently:

- `arm create --context-file PATH` may be repeated.
- `arm amend --context-file PATH` replaces the existing list; `--clear-context-files` removes it.
- `arm decompose-apply` accepts `context_files` arrays in plan JSON and preserves them in create/amend ops.
- `arm show --format json` includes the materialized `context_files` list.

The command surface should not infer context files from every scope entry. Auto-fill may propose likely design documents in a future helper, but persisted values must remain explicit so validation reflects human intent.

## Validation Policy

Current policy:

- Active tasks spanning three or more scope directories without `context_files` produce warning W5.
- Terminal tasks are skipped by W5 because their execution context no longer gates ready work.
- Missing CLI support keeps W5 advisory in normal validation. Strict or CI modes may report it, but should not block remediation that narrows scope instead.

Migration plan:

1. Add create/amend/decompose support for `context_files`.
2. Update planner templates and task import paths to populate the field when source docs constrain a task.
3. Keep W5 as a warning for one release after command support lands.
4. Revisit whether strict validation should promote W5 after dogfooding proves the command surface is complete.

## Consequences

Agents receive less noisy context because broad tasks can point at a small set of stable reference documents instead of relying on scope expansion. Operators get a clear way to satisfy validation without weakening scope. The tradeoff is additional task metadata maintenance, which should be handled by create/amend/decompose commands rather than by validator exceptions.
