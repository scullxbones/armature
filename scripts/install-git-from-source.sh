#!/usr/bin/env bash
# Build a pinned Git release from source (checksum-verified tarball) into --prefix.
# Supply-chain: version + URL + sha256 are required; no floating tags.
set -euo pipefail

VERSION=""
SHA256=""
PREFIX=""
URL=""
JOBS="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)"

usage() {
  cat <<'EOF' >&2
usage: install-git-from-source.sh --version X.Y.Z --sha256 HEX --prefix DIR [--url URL] [--jobs N]

Downloads git-<version>.tar.gz (default: kernel.org), verifies sha256, builds, and
installs into --prefix/bin/git. NO_GETTEXT / NO_TCLTK keep the build lean for CI.
EOF
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --sha256) SHA256="${2:-}"; shift 2 ;;
    --prefix) PREFIX="${2:-}"; shift 2 ;;
    --url) URL="${2:-}"; shift 2 ;;
    --jobs) JOBS="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "FAIL: unknown arg $1" >&2; usage ;;
  esac
done

if [[ -z "$VERSION" || -z "$SHA256" || -z "$PREFIX" ]]; then
  echo "FAIL: --version, --sha256, and --prefix are required" >&2
  usage
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "FAIL: --version must be X.Y.Z (got $VERSION)" >&2
  exit 1
fi

if [[ ! "$SHA256" =~ ^[0-9a-f]{64}$ ]]; then
  echo "FAIL: --sha256 must be 64 lowercase hex chars" >&2
  exit 1
fi

URL="${URL:-https://www.kernel.org/pub/software/scm/git/git-${VERSION}.tar.gz}"
TARBALL="git-${VERSION}.tar.gz"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/git-src.XXXXXX")"
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "Downloading $URL ..."
curl -fsSL --retry 3 --retry-delay 2 -o "$WORKDIR/$TARBALL" "$URL"

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

GOT="$(file_sha256 "$WORKDIR/$TARBALL")"
if [[ "$GOT" != "$SHA256" ]]; then
  echo "FAIL: checksum mismatch for $TARBALL" >&2
  echo "  expected $SHA256" >&2
  echo "  got      $GOT" >&2
  exit 1
fi
echo "OK: $TARBALL sha256 matches"

tar -xzf "$WORKDIR/$TARBALL" -C "$WORKDIR"
SRC="$WORKDIR/git-$VERSION"
if [[ ! -d "$SRC" ]]; then
  echo "FAIL: extracted tree missing $SRC" >&2
  exit 1
fi

mkdir -p "$PREFIX"
echo "Building git $VERSION into $PREFIX (jobs=$JOBS) ..."
make -C "$SRC" -j"$JOBS" prefix="$PREFIX" NO_GETTEXT=1 NO_TCLTK=1 all
make -C "$SRC" prefix="$PREFIX" NO_GETTEXT=1 NO_TCLTK=1 install

INSTALLED="$PREFIX/bin/git"
if [[ ! -x "$INSTALLED" ]]; then
  echo "FAIL: expected executable $INSTALLED" >&2
  exit 1
fi

raw="$("$INSTALLED" --version)"
echo "Installed: $raw"
case "$raw" in
  "git version ${VERSION}"|"git version ${VERSION}."*)
    ;;
  *)
    echo "FAIL: installed git version does not start with ${VERSION}: $raw" >&2
    exit 1
    ;;
esac
