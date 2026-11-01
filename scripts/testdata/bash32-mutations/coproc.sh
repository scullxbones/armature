#!/usr/bin/env bash
# Fixture: Bash 4+ coproc — must exit 0 under Bash 4+ and non-zero under 3.2.
set -euo pipefail
coproc _c { echo a; }
echo ok
