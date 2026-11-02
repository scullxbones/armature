#!/usr/bin/env bash
# Fixture: shell alias wrapping porcelain git must be banned outside helper.
set -euo pipefail
alias gs='git status'
gs --porcelain
