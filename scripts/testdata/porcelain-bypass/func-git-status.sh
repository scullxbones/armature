#!/usr/bin/env bash
# Fixture: gs() { git status; } must be banned.
set -euo pipefail
gs() { git status; }
gs
