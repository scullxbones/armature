#!/usr/bin/env bash
# Fixture: Bash 4.2+ [[ -v ]] — must fail under Bash 3.2 and 4.0/4.1;
# succeed under Bash ≥4.2.
set -euo pipefail
_v=1
[[ -v _v ]]
echo ok
