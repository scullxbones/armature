#!/usr/bin/env bash
# Fail if go-test left materialize state-dir artifacts untracked in the repo.
# Scoped to known leak names so unrelated WIP does not trip the gate.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"
# shellcheck disable=SC2016
mapfile -t dirty < <(git status --porcelain --untracked-files=all | awk '
  $1 ~ /^\?\?/ {
    path=$2
    n=split(path, parts, "/")
    base=parts[n]
    if (base == "checkpoint.json" || base == "index.json" || base == "ready.json" || base == "traceability.json" || base == "issues") {
      print path
    }
  }
')
if ((${#dirty[@]})); then
  echo "FAIL: go test left state-dir files in the working tree (explicit StateDir/TempDir required):" >&2
  printf '  %s\n' "${dirty[@]}" >&2
  exit 1
fi
echo "OK: no state-dir cwd leaks in git status --porcelain"
