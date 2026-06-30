# Finding: armature-planner skill does not show the plan JSON wrapper format

**Area:** documentation  
**Writer:** 5207ee28-cdd8-48e6-98dc-7da179d4a40d  
**Date:** 2026-06-22

## What I was trying to do

Writing a plan JSON for `arm decompose-apply --plan` while following the armature-planner skill. The skill's "Writing Good Plan JSON" section shows a complete well-formed task example and describes the required fields (`dod`, `scope`, `acceptance`, `notes`), but never shows the full plan file structure (the wrapper object).

## What happened

I wrote a plan with a custom structure (`{ "story": {...}, "tasks": [...] }`) based on inference. Running `arm decompose-apply --plan plan.json --dry-run` immediately failed:

```
Error: unsupported plan version: 0
```

I then ran `arm decompose-apply --example` to discover the actual required format: a top-level object with `"version": 1` and a flat `"issues"` array. After rewriting the plan in that format, the dry-run succeeded.

## Impact

- One failed attempt, one extra command, one rewrite of the plan file.
- Confidence was momentarily low: the error message `unsupported plan version: 0` is technically accurate but provides no hint about what the correct format looks like or where to find it.
- Time cost: ~2 minutes of rework.

## Evidence

- The skill mentions `arm decompose-apply --example` in the Quick Reference block but not in the "Writing Good Plan JSON" section where the format is taught.
- The skill shows a task-level JSON object in full detail but never wraps it in a `{ "version": 1, "issues": [...] }` envelope.
- Error text: `unsupported plan version: 0` — version field was absent so it defaulted to zero.

## Suggested fix

Add a minimal wrapper example to "Writing Good Plan JSON" immediately before the complete task example:

```json
{
  "version": 1,
  "title": "Optional plan title",
  "issues": [
    { ... }
  ]
}
```

Or add a note in the section intro: "The plan file must have `version: 1` and an `issues` array. Run `arm decompose-apply --example` to see the full envelope."
