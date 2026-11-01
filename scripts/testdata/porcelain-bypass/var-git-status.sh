#!/usr/bin/env bash
# Fixture: "$GIT" status must be banned outside helper.
set -euo pipefail
GIT=git
"$GIT" status --porcelain
