# Ops schema compatibility

**Closes:** GAP T5.2 / `TOPTIER-S6-T2`

## Contract

Every op log record is a JSON array:

```text
[op_type, target_id, timestamp, worker_id, payload, schema_version]
```

- **`schema_version` (index 5)** is the ops schema version for that record.
- Writers always emit `schema_version` equal to `ops.CurrentSchemaVersion` in this binary.
- Readers that see `schema_version` greater than they support **fail loudly** at parse time. An older `arm` must not silently skip, coerce, or half-replay newer ops.
- Records written before index 5 existed (legacy 5-element arrays) are treated as **version 1**. Missing `schema_version` defaults to 1; it is never treated as "unknown / ignore".
- Unknown trailing positions after `schema_version` remain ignorable (append-only field growth). Changing the meaning or type of an existing position requires a new schema version.

Scaffolding version (`# scaffolding-version:` in `ops/SCHEMA`) is separate: it versions generated repo files, not individual op records.

## Fail loud

`ops.ParseLine` calls `ops.CheckSchemaVersion`. On a newer version the error names both the seen version and the supported version and tells the operator to upgrade `arm`. Materialization does not begin on a log that fails this check.

## Fixture corpus

`internal/materialize/testdata/v1/` holds a committed v1 ops corpus and its golden materialized state. `TestReplayFixtureCorpus_REQ_TOPTIER-S6-T2` replays the corpus and requires a byte-identical golden. Every future schema version must keep that test green for the v1 corpus (replay v1 identically; do not rewrite history).

## Bumping the schema version

1. Document the change in this file (what broke, what readers must do).
2. Bump `ops.CurrentSchemaVersion`.
3. Keep `CheckSchemaVersion` rejecting anything above the new current.
4. Add or extend fixtures so prior versions still replay to their goldens.
5. Bump `ops.ScaffoldingVersion` when `GenerateSchema` text changes so bootstrap republishes `ops/SCHEMA`.

## Shim retirement (backward edge)

A backward-compatibility shim on a **migratable** surface (harness configs, on-disk layouts) may live for at most one minor release after its replacement lands: `bootstrap` / `doctor --fix` migrate adopters in that window, then the shim is deleted outright in the next minor — no indefinite carve-outs, and no plugin or adapter framework invented to host them. Pre-`v0.1.0` migratable shims (pre-marker Codex/Devin harness ownership, comma-joined scope entries) were deleted under this rule with zero external adopters.

**Grandfather (not a timed shim):** append-only ops history (Constitution I2) cannot be rewritten, so a load-only carve-out may be permanent when historical lines would otherwise be dropped. Boundary: in a slotted log named `<base>~<slot>.log`, a line whose `worker_id` equals the unslotted `<base>` is accepted on load; every other mismatch is still rejected. Writers stamp the full slotted stem (`worker.ResolveIdentity` with `ARMATURE_LOG_SLOT`) and are unchanged by this carve-out. The Harness Compatibility Contract (Next-Ten №08) remains the forward seam for future harness format changes and is not a substitute for this deletion policy.
