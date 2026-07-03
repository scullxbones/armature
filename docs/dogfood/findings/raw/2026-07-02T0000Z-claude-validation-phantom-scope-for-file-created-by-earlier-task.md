# Phantom-scope INFO fires for files a blocking task creates

- **Writer:** claude (planner, MIGH story load)
- **Area:** validation

## What I was trying to do

Give quality-gate task MIGH-T7 a scope covering all story files, including `cmd/armature/bootstrap_migration_matrix_test.go`, which upstream task MIGH-T6 creates (T6's scope marks it `(new)`).

## What happened

`arm validate` reports `INFO: phantom scope: ... on MIGH-T7 does not match any file`. The validator checks scope paths against the filesystem at plan time and doesn't recognize that a blocking task declares the same path as `(new)`.

## Impact

Minor (INFO only), but a planner following "no warnings/info noise" instincts either drops the file from the gate task's scope (weakening overlap detection) or accepts persistent noise. Cross-referencing `(new)` declarations across the DAG would remove the false positive.

## Evidence

`arm validate --ci` output for the MIGH plan, 2026-07-02.
