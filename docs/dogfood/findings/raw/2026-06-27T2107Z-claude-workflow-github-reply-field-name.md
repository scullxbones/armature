---
date: 2026-06-27
agent: claude
area: workflow
task: prfix — reply to Codex review comment on PR #59
tags: [github-api, review-threads, prfix]
---

# GitHub pull-request comment reply requires `in_reply_to`, not `in_reply_to_id`

## User Goal

Post a reply to an existing inline review comment (Codex bot, comment ID 3486284353) on PR #59 via `gh api`.

## Observed

First attempt used `--field "in_reply_to_id=3486284353"`. GitHub returned HTTP 422 with:

```
"in_reply_to_id" is not a permitted key.
```

Second attempt with `--field "in_reply_to=3486284353"` succeeded immediately.

## Impact

One extra API round-trip and a small pause to diagnose. The field name `in_reply_to_id` is what GitHub's REST docs and many code examples show; the actual accepted key is `in_reply_to`. Easy to hit on any prfix/review-reply flow.

## Evidence

```
gh api repos/scullxbones/armature/pulls/59/comments \
  --method POST \
  --field "body=..." \
  --field "in_reply_to_id=3486284353"
# → 422: "in_reply_to_id" is not a permitted key

gh api repos/scullxbones/armature/pulls/59/comments \
  --method POST \
  --field "body=..." \
  --field "in_reply_to=3486284353"
# → 200 OK
```

## Suggested Follow-Up

Update the `prfix` skill's finalize section to explicitly use `in_reply_to` (not `in_reply_to_id`) when posting review-thread replies via `gh api`.
