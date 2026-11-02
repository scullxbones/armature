#!/usr/bin/env bash
# Fixture: xargs→git (args before git) must be banned outside helper.
set -euo pipefail
echo status | xargs git
