#!/usr/bin/env bash
# Install PATH stubs for container engines so bare `docker`/`podman`/… fail.
# The sole wrapper (scripts/run-container-image.sh) resolves the real engine
# from a fixed absolute-path allowlist — no env override for the engine path.
# Intended as an early step in every CI job. Bash 3.2 compatible.
set -euo pipefail

ENGINES="docker podman buildx nerdctl ctr finch lima crictl"

SHADOW_DIR="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/armature-engine-shadow"
mkdir -p "$SHADOW_DIR"

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
  echo "ARM_ENGINE_SHADOW_DIR=$SHADOW_DIR" >>"$GITHUB_ENV"
fi

export PATH="$SHADOW_DIR:$PATH"
export ARM_ENGINE_SHADOW_DIR="$SHADOW_DIR"

echo "OK: engine PATH-shadow installed at $SHADOW_DIR"
