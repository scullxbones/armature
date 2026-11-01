#!/usr/bin/env bash
# Fixture: x=$(git status) || true must be banned.
set -euo pipefail
x=$(git status) || true
echo "$x"
