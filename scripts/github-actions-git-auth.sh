#!/usr/bin/env bash
# Source this file, then call github_actions_git_auth_export.
# Appends a github.com http.extraHeader for $GITHUB_TOKEN at the next free
# GIT_CONFIG_* index (Git ≥2.31). GitHub's git HTTPS endpoint rejects
# Authorization: Bearer with "invalid credentials"; the header is
# basic base64(x-access-token:TOKEN), the same scheme actions/checkout uses.
# The token is expanded only while xtrace is off. printf is a shell builtin
# and base64 reads the token on stdin, so it is not an argv element.

github_actions_git_auth_export() {
  local count idx next xtrace_was_on auth
  xtrace_was_on=0
  case "$-" in
    *x*) xtrace_was_on=1 ;;
  esac
  set +x
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    if [ "$xtrace_was_on" -eq 1 ]; then set -x; fi
    return 0
  fi
  count="${GIT_CONFIG_COUNT:-0}"
  case "$count" in
    ''|*[!0-9]*) count=0 ;;
  esac
  idx="$count"
  next=$((count + 1))
  auth=$(printf 'x-access-token:%s' "$GITHUB_TOKEN" | base64 | tr -d '\n')
  export GIT_CONFIG_COUNT="${next}"
  export "GIT_CONFIG_KEY_${idx}=http.https://github.com/.extraHeader"
  export "GIT_CONFIG_VALUE_${idx}=AUTHORIZATION: basic ${auth}"
  if [ "$xtrace_was_on" -eq 1 ]; then set -x; fi
}
