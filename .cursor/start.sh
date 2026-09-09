#!/usr/bin/env bash
# Per-boot reconciliation for Armature Cloud Agents.
#
# Runs on every environment start. The only per-boot work Armature needs is
# reconciling the managed git commit-signing config so the test suite's git
# subprocesses match a clean CI environment; see reconcile-git-signing.sh for
# the full rationale. Idempotent and safe to run on every boot.
set -euo pipefail

cd "$(dirname "$0")/.."
exec ./.cursor/reconcile-git-signing.sh "${CURSOR_WORKSPACE:-$(pwd)}"
