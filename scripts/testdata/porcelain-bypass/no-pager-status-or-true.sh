#!/usr/bin/env bash
# Fixture: git --no-pager status || true must be banned.
set -euo pipefail
git --no-pager status || true
