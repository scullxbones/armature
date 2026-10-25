#!/usr/bin/env bash
# Download a GitHub release tarball, verify sha256 against that release's
# published checksums file, and install one named binary into DEST.
#
# A checksum mismatch or a missing checksums entry fails the process.
# Does not fall back to go install or GOPROXY.
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/install-verified-github-release.sh --repo OWNER/NAME --version VER \
  --tarball NAME --checksums NAME --bin NAME --dest DIR

Tarball and checksums names may contain placeholders:
  {version}  version as given (--version)
  {ver}      version with a leading v stripped
  {os}       linux|darwin|windows
  {arch}     amd64|arm64

Optional:
  TARBALL_URL / CHECKSUMS_URL  override download URLs (tests)
  GITHUB_RELEASE_BASE          default https://github.com
EOF
    exit 2
}

REPO=""
VERSION=""
TARBALL_TMPL=""
CHECKSUMS_TMPL=""
BIN=""
DEST=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo) REPO="${2:-}"; shift 2 ;;
        --version) VERSION="${2:-}"; shift 2 ;;
        --tarball) TARBALL_TMPL="${2:-}"; shift 2 ;;
        --checksums) CHECKSUMS_TMPL="${2:-}"; shift 2 ;;
        --bin) BIN="${2:-}"; shift 2 ;;
        --dest) DEST="${2:-}"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "FAIL: unknown argument: $1" >&2; usage ;;
    esac
done

if [[ -z "$REPO" || -z "$VERSION" || -z "$TARBALL_TMPL" || -z "$CHECKSUMS_TMPL" || -z "$BIN" || -z "$DEST" ]]; then
    echo "FAIL: --repo, --version, --tarball, --checksums, --bin, and --dest are required" >&2
    usage
fi

detect_os() {
    local s
    s=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$s" in
        linux) echo linux ;;
        darwin) echo darwin ;;
        mingw*|msys*|cygwin*) echo windows ;;
        *) echo "FAIL: unsupported uname -s: $s" >&2; exit 1 ;;
    esac
}

detect_arch() {
    local m
    m=$(uname -m)
    case "$m" in
        x86_64|amd64) echo amd64 ;;
        aarch64|arm64) echo arm64 ;;
        *) echo "FAIL: unsupported uname -m: $m" >&2; exit 1 ;;
    esac
}

OS=$(detect_os)
ARCH=$(detect_arch)
VER="${VERSION#v}"

expand() {
    local s="$1"
    s="${s//\{version\}/$VERSION}"
    s="${s//\{ver\}/$VER}"
    s="${s//\{os\}/$OS}"
    s="${s//\{arch\}/$ARCH}"
    printf '%s\n' "$s"
}

TARBALL=$(expand "$TARBALL_TMPL")
CHECKSUMS=$(expand "$CHECKSUMS_TMPL")

GITHUB_RELEASE_BASE="${GITHUB_RELEASE_BASE:-https://github.com}"
TARBALL_URL="${TARBALL_URL:-${GITHUB_RELEASE_BASE%/}/${REPO}/releases/download/${VERSION}/${TARBALL}}"
CHECKSUMS_URL="${CHECKSUMS_URL:-${GITHUB_RELEASE_BASE%/}/${REPO}/releases/download/${VERSION}/${CHECKSUMS}}"

if ! command -v curl >/dev/null 2>&1; then
    echo "FAIL: curl is required to install $BIN" >&2
    exit 1
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

echo "Downloading $BIN@$VERSION (checksum-verified GitHub release)..."
echo "  tarball: $TARBALL_URL"
echo "  checksums: $CHECKSUMS_URL"

curl_get() {
    # No GOPROXY fallback. Retry is not used: a failed download must fail the job.
    curl -fsSL --retry 0 "$1" -o "$2"
}

curl_get "$CHECKSUMS_URL" "$WORKDIR/checksums"
curl_get "$TARBALL_URL" "$WORKDIR/$TARBALL"

file_sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

expected_hash() {
    local sums="$1" base="$2"
    awk -v b="$base" '
        {
            h=$1
            n=$2
            gsub(/^\*/, "", n)
            if (NF == 1 && $1 ~ /^[0-9a-fA-F]{64}$/) { print tolower(h); exit }
            if (n == b) { print tolower(h); exit }
        }
    ' "$sums"
}

HASH=$(expected_hash "$WORKDIR/checksums" "$TARBALL")
if [[ -z "$HASH" || ${#HASH} -ne 64 ]]; then
    echo "FAIL: checksums file has no sha256 entry for $TARBALL" >&2
    echo "checksums file:" >&2
    cat "$WORKDIR/checksums" >&2
    exit 1
fi

GOT=$(file_sha256 "$WORKDIR/$TARBALL")
if [[ "$GOT" != "$HASH" ]]; then
    echo "FAIL: checksum mismatch for $TARBALL" >&2
    echo "  expected: $HASH" >&2
    echo "  actual:   $GOT" >&2
    exit 1
fi
echo "OK: $TARBALL sha256 matches $CHECKSUMS"

mkdir -p "$WORKDIR/extract"
tar -xzf "$WORKDIR/$TARBALL" -C "$WORKDIR/extract"

FOUND=$(find "$WORKDIR/extract" -type f -name "$BIN" | head -n 1)
if [[ -z "$FOUND" ]]; then
    echo "FAIL: tarball $TARBALL does not contain binary $BIN" >&2
    find "$WORKDIR/extract" -type f >&2
    exit 1
fi

mkdir -p "$DEST"
install -m 0755 "$FOUND" "$DEST/$BIN"
echo "Installed $DEST/$BIN ($BIN@$VERSION)"
