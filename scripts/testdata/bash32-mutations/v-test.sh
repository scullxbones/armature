#!/usr/bin/env bash
# Fixture: Bash 4.2+ [[ -v ]] — must exit 0 under Bash 4+ and non-zero under 3.2.
set -euo pipefail
_v=1
[[ -v _v ]]
echo ok
