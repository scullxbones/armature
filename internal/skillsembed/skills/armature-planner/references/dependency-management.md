# Dependency Management

Use `arm link` to express ordering constraints between tasks.

```bash
arm link --source A --dep B    # A is blocked_by B (A runs after B completes)
arm unlink --source A --dep B  # remove a dependency
```

## When to Use `arm link`

- **Scope overlaps:** If two tasks touch the same file without an existing ordering
  relationship, one must run after the other. Run `arm validate` to surface scope
  overlap WARNINGs, then resolve each one with `arm link`.
- **Logical ordering:** Task A consumes the output of Task B (e.g. integration
  tests depend on the feature being implemented).
- **Avoiding collisions:** Tasks assigned to parallel workers must not have
  overlapping scope without an ordering dependency.

## Understanding Scope Overlap Detection

The scope-overlap checker automatically excludes certain valid overlap patterns so
you don't need `--force` to work around false positives:

### What the checker does NOT flag

1. **Parent/child containment:** A child task's scope is always a subset of its
   parent story's scope. Claiming or scheduling a child task against its own parent
   story does not produce a scope-overlap conflict.

2. **Transitive ordering:** If Task A blocks Task C via a chain (A → B → C or longer),
   they are correctly ordered and will not produce a scope-overlap WARNING, even if
   they share files. The checker computes the full transitive closure of blocking
   relationships, so multi-hop dependency chains suppress redundant warnings.

3. **Phantom-scope warnings:** If an upstream blocker declares a file as `(new)` in
   its scope, downstream tasks that reference that file will not emit a spurious
   phantom-scope INFO. The checker cross-references a blocker's declared `(new)`
   files before reporting legitimately-created files as phantom.

### What the checker DOES flag

- **Cross-story overlap:** Two tasks in different stories that share a scoped file
  with no ordering edge between them produce a WARNING. The checker scans for scope
  overlap across all story boundaries, not just within one story's task set.

- **Unordered same-file access:** Tasks without a direct or transitive blocking
  relationship that both touch the same file produce a WARNING.

## Checking for Overlaps

```bash
arm validate    # scope overlap WARNINGs appear here
```

For each WARNING, decide whether it's a real conflict:

- If the tasks are already ordered (A blocks B), the WARNING is redundant — no action
  needed.
- If they are in a parent/child relationship, the WARNING is a false positive — no
  action needed.
- If the overlap is across story boundaries, you likely need to order them.

To resolve a real conflict, add a link:
```bash
arm link --source LATER-TASK --dep EARLIER-TASK
arm validate    # re-run until all real WARNINGs are resolved
```

Reserve `--force` for exceptional cases (e.g. hotfixes or emergency parallel work).
Most scope-overlap WARNINGs are now correctly filtered, so reflexive `--force` use
is no longer necessary.
