---
name: prfix-review-threads-rest-404
description: gh api review_threads REST endpoint returns 404; resolving PR threads requires GraphQL
metadata:
  type: other
  area: workflow
---

## Finding

While running the prfix workflow on PR #63, the REST endpoint for PR review threads returned 404:

```
gh api repos/scullxbones/armature/pulls/63/review_threads
→ {"message": "Not Found", "documentation_url": "..."}
```

The response was 404 even though the PR exists and has review threads. Had to fall back to GraphQL to fetch thread node IDs needed for resolving conversations:

```graphql
{ repository(owner: "...", name: "...") {
    pullRequest(number: 63) {
      reviewThreads(first: 20) { nodes { id isResolved ... } }
    }
  }
}
```

Resolving a thread also requires GraphQL (`resolveReviewThread` mutation) — there is no REST endpoint for it.

## Impact

- Changed behavior: `gh api .../review_threads` is not a valid endpoint; the `gh api` wrapper hides the 404 error rather than surfacing it clearly
- Time spent: ~5 minutes diagnosing the 404 and switching to GraphQL
- The prfix skill does not document this requirement; it only says "post replies and resolve threads" without specifying the API path

## What would help

The prfix skill (or its finalize section) should note that resolving GitHub PR review threads requires GraphQL, not REST, and include the mutation template.
