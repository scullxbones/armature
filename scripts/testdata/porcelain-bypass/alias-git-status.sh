#!/usr/bin/env bash
# Fixture: var-held git command ($g status) must be banned outside helper.
set -euo pipefail
g=git
$g status --porcelain
