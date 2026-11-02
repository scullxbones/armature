#!/usr/bin/env bash
# Fixture: eval of porcelain git must be banned outside helper.
set -euo pipefail
eval "git status --porcelain"
