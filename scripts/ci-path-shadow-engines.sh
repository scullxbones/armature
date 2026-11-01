#!/usr/bin/env bash
# Install PATH stubs for container engines so bare `docker`/`podman`/… fail.
# Captures the real docker absolute path into ARM_DOCKER_BIN for the sole
# wrapper (scripts/run-container-image.sh). Intended as an early step in
# every CI job. Bash 3.2 compatible.
set -euo pipefail

ENGINES="docker podman buildx nerdctl ctr finch lima crictl"

SHADOW_DIR="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/armature-engine-shadow"
mkdir -p "$SHADOW_DIR"

# Resolve real docker BEFORE shadowing PATH.
REAL_DOCKER=""
if command -v docker >/dev/null 2>&1; then
  REAL_DOCKER="$(command -v docker)"
  # Prefer absolute path.
  case "$REAL_DOCKER" in
    /*) ;;
    *)
      REAL_DOCKER="$(cd "$(dirname "$REAL_DOCKER")" && pwd)/$(basename "$REAL_DOCKER")"
      ;;
  esac
fi

for eng in $ENGINES; do
  cat >"$SHADOW_DIR/$eng" <<'EOF'
#!/usr/bin/env bash
echo "FAIL: container engine blocked by CI PATH-shadow (use scripts/run-container-image.sh)" >&2
exit 1
EOF
  chmod +x "$SHADOW_DIR/$eng"
done

# Persist for subsequent steps (GitHub Actions) and current shell.
if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$SHADOW_DIR" >>"$GITHUB_PATH"
fi
if [ -n "${GITHUB_ENV:-}" ]; then
  if [ -n "$REAL_DOCKER" ]; then
    echo "ARM_DOCKER_BIN=$REAL_DOCKER" >>"$GITHUB_ENV"
  fi
  echo "ARM_ENGINE_SHADOW_DIR=$SHADOW_DIR" >>"$GITHUB_ENV"
fi

export PATH="$SHADOW_DIR:$PATH"
if [ -n "$REAL_DOCKER" ]; then
  export ARM_DOCKER_BIN="$REAL_DOCKER"
fi
export ARM_ENGINE_SHADOW_DIR="$SHADOW_DIR"

echo "OK: engine PATH-shadow installed at $SHADOW_DIR"
if [ -n "$REAL_DOCKER" ]; then
  echo "OK: ARM_DOCKER_BIN=$REAL_DOCKER"
else
  echo "OK: no host docker resolved (wrapper runs that need docker will fail closed)"
fi
