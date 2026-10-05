# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-10-05

First external cut. Install via GitHub Releases or `go install` (see README).
Supported platforms for running `arm`: Linux and macOS. Windows remains unsupported
for runtime use; CI still builds and compile-checks Windows as a packaging gate.

### Added

- CI OS matrix: build and test on ubuntu, macos, and windows; tag releases wait on that matrix ([TOPTIER-S6-T1](https://github.com/scullxbones/armature/pull/304)).
- Ops-schema versioning at array index 5 with fail-loud rejection of newer versions, plus a committed v1 replay fixture corpus ([TOPTIER-S6-T2](https://github.com/scullxbones/armature/pull/305)).
- README quickstart and install docs executed in CI ([TOPTIER-S7-T1](https://github.com/scullxbones/armature/pull/308)).
- Agent Output Contract for structured CLI envelopes (epic AOC).
- Armature Constitution (I1–I7) and related ADRs.
- `arm bootstrap` as the single repo setup path; collapsed `.armature/` ops layout (LNGHZN-S1).
- Transition-time delivery gate, managed worktree lifecycle, autonomic harness-hook heartbeats.
- Semantic conformance review (`arm review prepare` / `record` / `validate`).
- Deterministic worker runtime (`arm worker run`) and paved-road docs.
- Gate evidence, `make check-fast`, and context-report budgets.
- Recorded promotion path for `done → merged` (ADR 0022 / LNGHZN-S11 T1–T3).
- Community scaffolding: CONTRIBUTING, SECURITY, issue templates (TOPTIER-S9).

### Changed

- Product rename and CLI surface consolidation onto `arm` (from earlier Trellis naming).
- Subtractive release: park unused surface rather than grow indefinite shims; pre-v0.1.0 migratable legacy shims removed ([#309](https://github.com/scullxbones/armature/pull/309)).
- GoReleaser release notes now come from this CHANGELOG (see `.goreleaser.yaml` and the release workflow).

### Fixed

- Rollup no longer strands parents when a child is cancelled (TOPTIER-B1).
- Numerous delivery-gate, worktree, claim, and doctor fixes accumulated on the way to this cut.

## [0.0.2] - 2026-07-19

### Added

- Early tagged release via the GoReleaser pipeline (binaries and checksums on GitHub Releases).

## [0.0.1] - 2026-04-30

### Added

- First tagged release.

[Unreleased]: https://github.com/scullxbones/armature/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/scullxbones/armature/compare/v0.0.2...v0.1.0
[0.0.2]: https://github.com/scullxbones/armature/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/scullxbones/armature/releases/tag/v0.0.1
