#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARM_BIN="${ARM_BIN:-${ROOT}/bin/arm}"

die() {
	echo "promotion-ready-ci: $*" >&2
	exit 1
}

[[ -x "$ARM_BIN" ]] || {
	echo "promotion-ready-ci: building arm via make build" >&2
	make -C "$ROOT" build
}
[[ -x "$ARM_BIN" ]] || die "arm binary not executable at $ARM_BIN"

# Unit + CLI fixture proof for ADR 0026 N1 poka-yoke (issue #480).
cd "$ROOT"
go test ./internal/promotion/ ./cmd/armature/ -count=1 -run 'REQ_480'

echo "promotion-ready-ci: OK"
