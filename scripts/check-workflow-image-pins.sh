#!/usr/bin/env bash
# Thin wrapper: YAML-parse workflow image pins via scripts/workflowimagepins.
set -euo pipefail
ROOT="${1:-.}"
ROOT="$(cd "$ROOT" && pwd)"
cd "$ROOT"
go run ./scripts/workflowimagepins -selftest "$ROOT"
go run ./scripts/workflowimagepins "$ROOT"
