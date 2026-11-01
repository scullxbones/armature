#!/usr/bin/env bash
# Static container-image pin scan via scripts/workflowimagepins.
# Ref/digest findings are BLOCKING; engine-word findings are advisory only.
# Fail-closed runtime enforcement is CI PATH-shadow
# (scripts/ci-path-shadow-engines.sh) + scripts/run-container-image.sh.
# Selftest remains fail-closed (fixture twins).
set -euo pipefail
ROOT="${1:-.}"
ROOT="$(cd "$ROOT" && pwd)"
cd "$ROOT"
go run ./scripts/workflowimagepins -selftest "$ROOT"
go run ./scripts/workflowimagepins "$ROOT"
