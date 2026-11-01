#!/usr/bin/env bash
# Fixture: git status|cat must be banned.
set -euo pipefail
git status|cat
