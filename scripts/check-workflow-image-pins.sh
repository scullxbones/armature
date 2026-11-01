#!/usr/bin/env bash
# Static ADVISORY image-pin scan via scripts/workflowimagepins.
# Fail-closed runtime enforcement is CI PATH-shadow
# (scripts/ci-path-shadow-engines.sh) + scripts/run-container-image.sh.
# Selftest remains fail-closed (fixture twins). Repo scan is -advisory.
set -euo pipefail
ROOT="${1:-.}"
ROOT="$(cd "$ROOT" && pwd)"
cd "$ROOT"
go run ./scripts/workflowimagepins -selftest "$ROOT"
go run ./scripts/workflowimagepins -advisory "$ROOT"
