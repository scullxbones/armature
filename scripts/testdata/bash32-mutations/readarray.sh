#!/usr/bin/env bash
# Fixture: Bash 4+ readarray — must fail under Bash 3.2.
readarray -t _x < <(echo a)
echo ok
