#!/usr/bin/env bash
# Fixture: Bash 4+ coproc — must fail under Bash 3.2.
coproc _c { echo a; }
echo ok
