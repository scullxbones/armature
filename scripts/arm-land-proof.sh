#!/usr/bin/env bash
# Hermetic REQ_481 proof for check jobs (no network host land).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export ARM_LAND_PROOF_ONLY=1
exec bash "$ROOT/scripts/arm-land-ci.sh"
