#!/usr/bin/env bash
# Sole container-engine entrypoint for workflows, composite actions, and make.
# Usage: run-container-image.sh <manifest-key> [known-run-flags...] -- [cmd...]
#
# - Key is the only image input (looked up in scripts/container-images.manifest).
# - Manifest path is fixed (no ARM_CONTAINER_IMAGE_MANIFEST override).
# - Invokes the real engine by absolute path (ARM_DOCKER_BIN or /usr/bin/docker).
# - Image is placed last before the command, after a `--` separator.
# - Bash 3.2 compatible (macOS /bin/bash).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="$ROOT/scripts/container-images.manifest"

usage() {
  echo "FAIL: usage: run-container-image.sh <manifest-key> [run-flags...] -- [cmd...]" >&2
}

if [ "$#" -lt 1 ]; then
  usage
  exit 2
fi

# Reject flags/positionals before the key — key must be argv[1].
case "$1" in
  -*)
    echo "FAIL: manifest key must be the first argument (got $1)" >&2
    exit 2
    ;;
esac

KEY="$1"
shift

# Key is the only image input — reject image-shaped / glob keys.
case "$KEY" in
  ''|*/*|*:|*@*|*[*]*)
    echo "FAIL: invalid manifest key $KEY (key is the only image input)" >&2
    exit 2
    ;;
esac
case "$KEY" in
  *[!A-Za-z0-9._-]*)
    echo "FAIL: invalid manifest key charset: $KEY" >&2
    exit 2
    ;;
esac

if [ ! -f "$MANIFEST" ]; then
  echo "FAIL: container image manifest missing: $MANIFEST" >&2
  exit 1
fi

IMAGE=""
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    ''|\#*) continue ;;
  esac
  k="${line%%[$'\t' ]*}"
  rest="${line#"$k"}"
  rest="${rest#"${rest%%[!$' \t']*}"}"
  if [ "$k" = "$KEY" ]; then
    IMAGE="$rest"
    break
  fi
done <"$MANIFEST"

if [ -z "$IMAGE" ]; then
  echo "FAIL: manifest key not found: $KEY" >&2
  exit 1
fi

# Validate name@sha256:<64-hex> (reject alpine:latest, bare digests, globs).
case "$IMAGE" in
  *[\'\"\$\`\\*]*)
    echo "FAIL: manifest image contains unsafe characters: $IMAGE" >&2
    exit 1
    ;;
esac
img_ok=0
case "$IMAGE" in
  *@sha256:*)
    name="${IMAGE%@sha256:*}"
    dig="${IMAGE##*@sha256:}"
    case "$name" in
      ''|*@*|*:) ;;
      *)
        case "$dig" in
          *[!0-9a-f]*|'' ) ;;
          *)
            if [ "${#dig}" -eq 64 ]; then
              img_ok=1
            fi
            ;;
        esac
        ;;
    esac
    ;;
esac
if [ "$img_ok" -ne 1 ]; then
  echo "FAIL: manifest value must be name@sha256:<64-hex>, got $IMAGE" >&2
  exit 1
fi

# Known docker-run flags (boolean vs value-taking).
flag_bool() {
  case "$1" in
    --rm|--tty|-t|--interactive|-i|--detach|-d|--init|--privileged|--read-only|--quiet|-q|--pull=*)
      return 0
      ;;
  esac
  return 1
}

flag_value() {
  case "$1" in
    --entrypoint|-v|--volume|--mount|-w|--workdir|-e|--env|--env-file|-u|--user|--name|--network|--net|--platform|-p|--publish|--label|--cidfile|--memory|-m|--cpus|--cpu-shares|--restart|--runtime|--security-opt|--device|--add-host|--dns|--health-cmd|--health-interval)
      return 0
      ;;
  esac
  return 1
}

# Bash 3.2 arrays for quoted flag passing. Validate before resolving docker.
flags=()
cmd_set=0
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--" ]; then
    shift
    cmd_set=1
    break
  fi
  arg="$1"
  case "$arg" in
    -*)
      if flag_bool "$arg"; then
        flags+=("$arg")
        shift
        continue
      fi
      case "$arg" in
        --*=*)
          base="${arg%%=*}"
          if flag_value "$base" || flag_bool "$arg"; then
            flags+=("$arg")
            shift
            continue
          fi
          ;;
      esac
      if flag_value "$arg"; then
        if [ "$#" -lt 2 ]; then
          echo "FAIL: flag $arg requires a value" >&2
          exit 2
        fi
        flags+=("$arg" "$2")
        shift 2
        continue
      fi
      echo "FAIL: unknown/disallowed docker-run flag: $arg" >&2
      exit 2
      ;;
    *)
      echo "FAIL: unexpected positional before --: $arg (image comes only from manifest key)" >&2
      exit 2
      ;;
  esac
done

if [ "$cmd_set" -ne 1 ]; then
  echo "FAIL: missing -- before command (image is inserted after --)" >&2
  exit 2
fi

# Resolve absolute docker binary (never bare PATH lookup under PATH-shadow).
DOCKER_BIN="${ARM_DOCKER_BIN:-}"
if [ -z "$DOCKER_BIN" ]; then
  for cand in /usr/bin/docker /usr/local/bin/docker; do
    if [ -x "$cand" ]; then
      DOCKER_BIN="$cand"
      break
    fi
  done
fi
if [ -z "$DOCKER_BIN" ] || [ ! -x "$DOCKER_BIN" ]; then
  echo "FAIL: real docker binary not found (set ARM_DOCKER_BIN to an absolute path)" >&2
  exit 1
fi
case "$DOCKER_BIN" in
  /*) ;;
  *)
    echo "FAIL: ARM_DOCKER_BIN must be absolute, got $DOCKER_BIN" >&2
    exit 1
    ;;
esac

# Image last after -- : engine sees: run [flags...] -- IMAGE [cmd...]
exec "$DOCKER_BIN" run "${flags[@]}" -- "$IMAGE" "$@"
