---
date: 2026-09-21
agent: loops
area: integration
task: publish MATENC ops via Cloud Agent
tags: [cloud-agent, attach, zip, dogfood]
---

# CloudAgent launch rejects .zip/.tgz file attachments

## User Goal

Hand a 13-file `_armature` delta to a Cloud Agent as one archive because the box PAT cannot git-push.

## Observed

`CloudAgent` launch with `files: [{url: file:///…/ops-bundle.tgz}]` or `.zip` failed: "Could not read the file… check it actually exists" even though `ls` showed the file. Attaching a `.md` and individual `.log`/`.cache`/`.json` files succeeded.

## Impact

Had to attach 13 files instead of one archive; more fragile mapping instructions.

## Evidence

Launch errors for tgz/zip; successful launch bc-10b47a7b with per-file attaches.

## Suggested Follow-Up

Allow zip/tar/gz on CloudAgent `files`, or document the restriction in coordinator/planner dogfood notes.
