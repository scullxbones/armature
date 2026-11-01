#!/usr/bin/env bash
# Sole docker entrypoint for workflows, composite actions, and make.
# Usage: run-container-image.sh <manifest-key> [docker-run-flags...] -- [cmd...]
# Looks up <manifest-key> in scripts/container-images.manifest and refuses
# anything else. Bash 3.2 compatible.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="${ARM_CONTAINER_IMAGE_MANIFEST:-$ROOT/scripts/container-images.manifest}"

if [ "$#" -lt 1 ]; then
  echo "FAIL: usage: run-container-image.sh <manifest-key> [docker-run-flags...] -- [cmd...]" >&2
  exit 2
fi

KEY="$1"
shift

if [ ! -f "$MANIFEST" ]; then
  echo "FAIL: container image manifest missing: $MANIFEST" >&2
  exit 1
fi

IMAGE=""
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    ''|\#*) continue ;;
  esac
  # key<TAB>image  or  key<spaces>image
  k="${line%%[$'\t' ]*}"
  rest="${line#"$k"}"
  rest="${rest#"${rest%%[!$' \t']*}"}"
  if [ "$k" = "$KEY" ]; then
    IMAGE="$rest"
    break
  fi
done <"$MANIFEST"

if [ -z "$IMAGE" ]; then
  echo "FAIL: manifest key not found: $KEY (manifest: $MANIFEST)" >&2
  exit 1
fi

# Split flags before -- from the command after --.
flags=""
cmd_set=0
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--" ]; then
    shift
    cmd_set=1
    break
  fi
  flags="$flags $1"
  shift
done

# shellcheck disable=SC2086
if [ "$cmd_set" -eq 1 ]; then
  exec docker run $flags "$IMAGE" "$@"
fi
exec docker run $flags "$IMAGE"
