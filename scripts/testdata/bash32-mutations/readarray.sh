#!/usr/bin/env bash
# Fixture: Bash 4+ readarray — must exit 0 under Bash 4+ and non-zero under 3.2.
# set -e is required: without it, a missing builtin (exit 127) is ignored and
# `echo ok` makes the script look successful under 3.2.
set -euo pipefail
readarray -t _x < <(echo a)
echo ok
