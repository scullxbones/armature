# Tooling Integration Gaps

External tools (gh, gopls) expose unexpected schema mismatches or workspace discovery behavior that breaks workflows without clear signals.

## Findings

- [gh pr view JSON field unavailability](../../raw/2026-06-22T000000Z-5207ee28-tooling-gh-json-field-headsha.md) — `gh pr view --json headSha` fails because the field doesn't exist in gh's PR schema; workaround is `git rev-parse HEAD`.
- [gopls workspace discovery with git worktrees](../../raw/2026-06-22-tooling-worktree-lsp-workspace-warnings.md) — gopls picks up `go.mod` files in sibling worktree paths and surfaces false compiler errors. Verification with `go build ./...` in the primary repo confirms the build is actually sound.

## Pattern

When skills or workflows integrate with external tools, schema assumptions or workspace behavior can diverge from documentation or intuitive expectations. The errors appear real but are context-specific artifacts.

## Mitigations

For gh commands: Document fallback paths (e.g., use git or raw API calls) when gh JSON schema lacks required fields.

For gopls: Add a `go.work` file excluding worktree paths, or document that worktree LSP errors are expected and should be verified against primary-repo builds.
