# Armature — Agent Setup

Armature is a git-native work orchestration system: state is append-only ops materialized into task state under `.armature/`, with no external DB or server.

Go project. Build with `make`, not raw `go build`.

## First-time setup

```bash
make install                                 # build → ~/.local/bin/arm
arm bootstrap                                # init repo + deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
arm worker-init --check || arm worker-init   # register worker identity once per clone (don't rerun without --check)
```

## Details

- [Workflow & operating model](docs/agents/workflow.md)
- [Quality gates — TDD, `make check`, coverage/mutation thresholds](docs/agents/quality-gates.md)
- [Dogfood findings capture](docs/agents/dogfood-findings.md)
- [Repo-local skills](docs/agents/skills.md)

## Canonical references

- `docs/commands.md` — CLI surface
- `docs/harness-hook.md` — harness integration
- `docs/design/architecture.md` — architecture and repo model
- `CONTEXT.md` — domain glossary
