# Trellis — Claude Code Rules

## TDD is mandatory

Write failing tests before writing implementation code. No exceptions.

1. Write a failing test that captures the requirement.
2. Write the minimum code to make it pass.
3. Refactor if needed.

Never commit implementation code without a corresponding test.

## `make check` must be green before every commit and push

Run `make check` and confirm all four stages pass:

```
make check   # lint + test + coverage-check (≥80%) + mutate
```

All stages must be green. Do not push with a failing `make check`. Do not ignore or suppress failures — fix them.

## No bypassing lint or coverage

- Do not add `//nolint` unless the suppression is genuinely justified (e.g. intentional error discard in a cleanup path).
- Do not lower the 80% coverage threshold.
- Do not skip mutation testing.
