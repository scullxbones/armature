#!/usr/bin/env bash
# Fixture: Bash 4.2+ [[ -v ]] — must fail under Bash 3.2.
_v=1
[[ -v _v ]] && echo ok
