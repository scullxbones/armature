#!/usr/bin/env bash
# Sole helper for porcelain-related git invocations used by
# scripts/check-test-porcelain.sh. Other scripts must not call
# `git status` / `git hash-object` directly (enforced by
# scripts/check-porcelain-failclosed.sh).
#
# Source this file; do not exec. Bash 3.2 compatible.
# shellcheck shell=bash

porcelain_git_rev_parse() {
  git rev-parse "$@"
}

porcelain_git_status_z() {
  # NUL-separated porcelain; caller must capture via a temp file (not
  # process substitution) so a non-zero exit propagates under set -e.
  git status --porcelain -z --untracked-files=all "$@"
}

porcelain_git_hash_object() {
  # Fail-closed: callers must not soft-fallback on failure.
  git hash-object "$@"
}
