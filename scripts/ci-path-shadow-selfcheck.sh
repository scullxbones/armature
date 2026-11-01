#!/usr/bin/env bash
# Prove PATH-shadow: bare engine invocations fail; optional wrapper smoke when
# ARM_DOCKER_BIN is set. Bash 3.2 compatible.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail=0

# Ensure shadow is on PATH (idempotent if already installed).
if [ -z "${ARM_ENGINE_SHADOW_DIR:-}" ] || [ ! -d "${ARM_ENGINE_SHADOW_DIR:-}" ]; then
  # shellcheck source=scripts/ci-path-shadow-engines.sh
  . "$ROOT/scripts/ci-path-shadow-engines.sh"
fi

set +e
out="$(docker run --rm alpine:latest true 2>&1)"
st=$?
set -e
if [ "$st" -eq 0 ]; then
  echo "FAIL: bare docker run succeeded under PATH-shadow: $out" >&2
  fail=1
else
  echo "OK: bare docker run failed under PATH-shadow (exit $st)"
fi

set +e
out="$(podman run --rm alpine:latest true 2>&1)"
st=$?
set -e
if [ "$st" -eq 0 ]; then
  echo "FAIL: bare podman run succeeded under PATH-shadow: $out" >&2
  fail=1
else
  echo "OK: bare podman run failed under PATH-shadow (exit $st)"
fi

# Wrapper smoke (opt-in): prove absolute-path invocation works. Enabled on
# check-bash32 via ARM_PATH_SHADOW_WRAPPER_SMOKE=1; other jobs still prove
# bare engine invocations fail under PATH-shadow.
if [ "${ARM_PATH_SHADOW_WRAPPER_SMOKE:-0}" = "1" ]; then
  if [ -z "${ARM_DOCKER_BIN:-}" ] || [ ! -x "${ARM_DOCKER_BIN}" ]; then
    echo "FAIL: wrapper smoke requested but ARM_DOCKER_BIN missing/unusable" >&2
    fail=1
  else
    set +e
    out="$(bash "$ROOT/scripts/run-container-image.sh" bash-3.2.57 --rm --entrypoint sh -- -c 'echo wrapper_ok' 2>&1)"
    st=$?
    set -e
    if [ "$st" -ne 0 ]; then
      echo "FAIL: run-container-image.sh failed with ARM_DOCKER_BIN set: $out" >&2
      fail=1
    elif ! printf '%s\n' "$out" | grep -q 'wrapper_ok'; then
      echo "FAIL: wrapper missing wrapper_ok: $out" >&2
      fail=1
    else
      echo "OK: run-container-image.sh works via absolute ARM_DOCKER_BIN"
    fi
  fi
else
  echo "OK: skip wrapper smoke (ARM_PATH_SHADOW_WRAPPER_SMOKE!=1)"
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: PATH-shadow self-check"
