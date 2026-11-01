#!/usr/bin/env bash
# Fixture: git status; must be banned.
set -euo pipefail
git status;
true
