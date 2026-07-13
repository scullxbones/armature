# ADR: Depguard Boundaries for Deep Modules

## Principles touched

none

Seven packages are designated deep modules — narrow public interfaces hiding substantial implementation — and are protected by strict-mode depguard allow lists in `.golangci.yml`: `ops`, `claim`, `traceability`, `materialize`, `sources`, `validate`, and `output`. `dag` and `issuetype` are additionally guarded as truly pure packages (no internal imports at all). All rules are immediately green against the current codebase.

The distinction between "pure" (dag, issuetype — no internal imports) and "port-clean" (ops, claim, traceability, materialize, sources — may use `adapters` as the I/O port abstraction) reflects a hexagonal architecture where `internal/adapters` is the secondary port layer. Using adapters does not make a package imperative; it uses a controlled, testable abstraction boundary.

Strict-mode allow lists were chosen over deny lists for the port-clean and consumer packages because deep modules accrue invisible coupling — strict mode surfaces any new internal import at lint time rather than in review. Rules exclude test files (`!**/*_test.go`) so that test infrastructure (testify, property-based testing libraries) does not need to appear in every allow list.

The provider implementations (Confluence, SharePoint, Filesystem) remain in `internal/sources` as secondary adapters implementing the `Provider` port defined there. They are correctly placed: they implement a port, they only import stdlib and `adapters`, and `sources-boundary` already enforces that constraint.
