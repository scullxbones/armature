#!/usr/bin/env bash
# Fixture: absolute-path docker must be banned outside the wrapper.
set -euo pipefail
/usr/bin/docker run --rm alpine:latest true
