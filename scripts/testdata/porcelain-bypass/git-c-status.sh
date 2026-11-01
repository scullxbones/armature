#!/usr/bin/env bash
# Fixture: git -c core.quotepath=false status must be banned outside helper.
set -euo pipefail
git -c core.quotepath=false status --porcelain
