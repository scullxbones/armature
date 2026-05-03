# Dependency Management

Use `arm link` to express ordering constraints between tasks.

```bash
arm link --source A --dep B    # A is blocked_by B (A runs after B completes)
arm unlink --source A --dep B  # remove a dependency
```

## When to Use `arm link`

- **Scope overlaps:** If two tasks touch the same file, one must run after the
  other. Run `arm validate` to surface scope overlap WARNINGs, then resolve
  each one with `arm link`.
- **Logical ordering:** Task A consumes the output of Task B (e.g. integration
  tests depend on the feature being implemented).
- **Avoiding collisions:** Tasks assigned to parallel workers must not have
  overlapping scope without an ordering dependency.

## Checking for Overlaps

```bash
arm validate    # scope overlap WARNINGs appear here
```

For each WARNING, decide which task runs first and add the link:
```bash
arm link --source LATER-TASK --dep EARLIER-TASK
arm validate    # re-run until all WARNINGs are resolved
```
