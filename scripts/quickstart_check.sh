#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
README="${ROOT}/README.md"
ARM_BIN="${ARM_BIN:-${ROOT}/bin/arm}"

die() {
	echo "quickstart_check: $*" >&2
	exit 1
}

[[ -f "$README" ]] || die "missing $README"
[[ -x "$ARM_BIN" ]] || {
	echo "quickstart_check: building arm via make build" >&2
	make -C "$ROOT" build
}
[[ -x "$ARM_BIN" ]] || die "arm binary not executable at $ARM_BIN"

grep -q 'go install github.com/scullxbones/armature/cmd/armature' "$README" \
	|| die "README Installation must document go install"
grep -Eqi 'GitHub Releases|github.com/scullxbones/armature/releases' "$README" \
	|| die "README Installation must document released binaries"

extract_quickstart_bash() {
	awk '
		BEGIN { in_section = 0; in_fence = 0 }
		/^## 5-Minute Quickstart[[:space:]]*$/ { in_section = 1; next }
		in_section && /^##[[:space:]]/ { exit }
		in_section && /^```bash[[:space:]]*$/ { in_fence = 1; next }
		in_section && in_fence && /^```[[:space:]]*$/ { in_fence = 0; print ""; next }
		in_section && in_fence { print }
	' "$README"
}

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/armature-quickstart.XXXXXX")"
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

QUICKSTART_FILE="$WORKDIR/quickstart.sh"
extract_quickstart_bash >"$QUICKSTART_FILE"
[[ -s "$QUICKSTART_FILE" ]] || die "no bash fences found under ## 5-Minute Quickstart"

ORIGIN="$WORKDIR/origin.git"
REPO="$WORKDIR/repo"
git init --bare "$ORIGIN" >/dev/null
git clone "$ORIGIN" "$REPO" >/dev/null 2>&1
cd "$REPO"

git config user.email "quickstart-check@example.com"
git config user.name "Quickstart Check"
git config commit.gpgsign false

mkdir -p docs
cat >docs/requirements.md <<'EOF'
# Demo requirements

Ship a hello greeting.
EOF
git add docs/requirements.md
git commit -m "chore: seed requirements for quickstart check" >/dev/null
git push -u origin HEAD:main >/dev/null

export PATH="$(dirname "$ARM_BIN"):$PATH"
hash -r
command -v arm >/dev/null || die "arm not on PATH after prepending $(dirname "$ARM_BIN")"

bash -euo pipefail "$QUICKSTART_FILE"

echo "quickstart_check: OK"
