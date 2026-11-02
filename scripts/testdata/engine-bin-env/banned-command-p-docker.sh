#!/usr/bin/env bash
# Fixture: command -p docker must be banned outside the wrapper.
set -euo pipefail
command -p docker run --rm alpine:latest true
