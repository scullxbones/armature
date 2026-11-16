#!/usr/bin/env bash
# ADR 0026 N1 (#483): hermetic fixture-repo squash-merge proof bar.
# Covers REQ_480 (poka-yoke) + REQ_481 (land) + REQ_482 (jidoka) + REQ_483
# (already-promoted dry-run + inventory). No network host land.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

go test ./cmd/armature/ ./internal/promotion/ ./internal/ops/ ./internal/adapters/ ./internal/externalref/ \
	-count=1 -run 'REQ_480|REQ_481|REQ_482|REQ_483'

echo "n1-proof: OK"
