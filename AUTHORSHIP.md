# Authorship and copyright for agent-authored commits

This document states how Armature attributes commits that an autonomous agent
authors (and a human merges), and which license terms apply to those
contributions. It closes narrow-gaps-addendum G6.1 (sequencing item 23 /
`TOPTIER-S16`).

It is about **legal attribution of repository content**, not about operational
trust between writers in the ops log.

## Commit trailer / co-author convention

Agent-authored delivery commits use Git trailers so attribution stays visible
in `git log` and on GitHub:

1. **Git author / committer** — Prefer the responsible human maintainer (or the
   human who merges the change) as the commit `Author`. Do not set the agent as
   the sole Git author for merged history.
2. **Agent co-author trailer** — Every commit whose content was substantially
   produced by an agent MUST include a `Co-authored-by` trailer naming the
   agent or harness identity, for example:
   ```
   Co-authored-by: Cursor Agent <cursoragent@cursor.com>
   ```
   Use the harness's documented identity when one exists; otherwise use a
   stable `Name <email>` that identifies the agent or model family.
3. **Human co-author trailer** — When a human reviews, directs, or merges the
   change, add a second trailer for that person:
   ```
   Co-authored-by: Example Maintainer <maintainer@example.com>
   ```
4. **Subject line** — Keep the ordinary conventional-commit subject from
   [docs/conventions.md](docs/conventions.md) (`<type>(<ISSUE-ID>): …`). Trailers
   belong in the commit message footer after a blank line, not in the subject.
5. **Optional session trailer** — Harnesses MAY add an extra trailer such as
   `Claude-Session: <id>` or a Cursor session id for forensic correlation. That
   trailer is optional and does not replace `Co-authored-by`.

Example footer:

```
docs(TOPTIER-S16-T1): document agent authorship and license terms

Co-authored-by: Cursor Agent <cursoragent@cursor.com>
Co-authored-by: Example Maintainer <maintainer@example.com>
```

Human-only commits need no agent trailer. Purely mechanical bot commits (CI
bots that do not author product code) follow their existing bot identity and
are out of scope here.

## License terms for agent-generated contributions

Armature is licensed under the [Apache License 2.0](LICENSE).

By contributing — whether the text was typed by a human, produced by an agent
under human direction, or a mix of both — you agree that:

1. **Same license** — The contribution is submitted under Apache License 2.0,
   the same terms as the rest of the Work.
2. **Inbound = outbound** — Agent-generated content merged into this repository
   is licensed to recipients under Apache-2.0 with no special carve-out,
   dual license, or withheld rights for "AI-generated" material.
3. **Contributor responsibility** — The human who opens or merges the pull
   request is responsible for confirming they have the right to submit the
   contribution under those terms (including any obligations imposed by the
   agent provider or their employer).
4. **No separate agent copyright claim** — Attribution via `Co-authored-by`
   documents assistance; it does not create a separate license grant or a
   claim that agent-generated lines are unlicensed or public-domain. Once
   merged, those lines are part of the Apache-2.0 Work.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[LICENSE](LICENSE) for the full license text.
