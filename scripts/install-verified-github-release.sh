#!/usr/bin/env bash
# Install one pinned GitHub release binary after sha256 verification.
# Transient HTTP failures (including 504) are retried with curl's exponential
# backoff, bounded by ARMATURE_TOOL_FETCH_RETRIES and
# ARMATURE_TOOL_FETCH_RETRY_MAX_TIME. Verified tarballs are cached at
# $cache/$owner/$name/$version/$sha256/$tarball. A cache hit is installed
# only when the bytes still match that sha256. There is no unpinned URL
# and no skip-verification path.
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
  ARMATURE_GITHUB_API          asset API base, 0 to skip; default
                               https://api.github.com when the release base is
                               https://github.com. Same tag and filename only.
  ARMATURE_TOOL_CACHE          cache directory, or 0 to disable
                               default $XDG_CACHE_HOME/armature/github-releases
                               or $HOME/.cache/armature/github-releases
  ARMATURE_TOOL_FETCH_RETRIES  extra attempts after the first (default 5, max 8)
  ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
                               seconds for the retry budget (default 120, max 300)
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

require_uint() {
    local name="$1" value="$2" min="$3" max="$4" n
    case "$value" in
        ''|*[!0-9]*)
            echo "FAIL: $name must be an integer (got $value)" >&2
            exit 1
            ;;
    esac
    n=$((10#$value))
    if [[ "$n" -lt "$min" || "$n" -gt "$max" ]]; then
        echo "FAIL: $name must be between $min and $max (got $value)" >&2
        exit 1
    fi
    printf '%s\n' "$n"
}

FETCH_RETRIES=$(require_uint ARMATURE_TOOL_FETCH_RETRIES "${ARMATURE_TOOL_FETCH_RETRIES:-5}" 1 8)
FETCH_RETRY_MAX_TIME=$(require_uint ARMATURE_TOOL_FETCH_RETRY_MAX_TIME "${ARMATURE_TOOL_FETCH_RETRY_MAX_TIME:-120}" 1 300)

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

echo "Downloading $BIN@$VERSION (checksum-verified GitHub release)..."
echo "  tarball: $TARBALL_URL"
echo "  checksums: $CHECKSUMS_URL"
echo "  retry: ${FETCH_RETRIES} extra attempts, exponential backoff, budget ${FETCH_RETRY_MAX_TIME}s"

# --retry covers timeouts and HTTP 408/429/500/502/503/504.
# --retry-delay is omitted so curl keeps its exponential backoff.
# Prints the HTTP status on stdout. Body goes to dest. Empty status on
# a transport failure after retries.
curl_retry() {
    local dest="$1" url="$2"
    shift 2
    curl -sS -f \
        --retry "$FETCH_RETRIES" \
        --retry-max-time "$FETCH_RETRY_MAX_TIME" \
        --retry-connrefused \
        --max-time 120 \
        "$@" \
        -o "$dest" \
        -w '%{http_code}' \
        "$url" || true
}

curl_get() {
    local code
    code=$(curl_retry "$2" "$1")
    [[ "$code" == "200" ]]
}

# GitHub answers secondary rate limits with 403 or 429. curl --retry does not
# treat those as transient, and one 403 must not abandon a pinned asset.
curl_resilient() {
    local dest="$1" url="$2"
    shift 2
    local attempt=1 code backoff
    backoff=$(require_uint ARMATURE_TOOL_FETCH_BACKOFF_SECS "${ARMATURE_TOOL_FETCH_BACKOFF_SECS:-2}" 0 30)
    while [[ "$attempt" -le 4 ]]; do
        code=$(curl_retry "$dest" "$url" "$@")
        if [[ "$code" == "200" ]]; then
            printf '%s\n' 200
            return 0
        fi
        if [[ "$code" != "403" && "$code" != "429" ]]; then
            printf '%s\n' "$code"
            return 0
        fi
        if [[ "$attempt" -eq 4 ]]; then
            printf '%s\n' "$code"
            return 0
        fi
        echo "WARN: HTTP $code (attempt $attempt); backing off" >&2
        rm -f "$dest"
        if [[ "$backoff" -gt 0 ]]; then
            sleep $((attempt * backoff))
        fi
        attempt=$((attempt + 1))
    done
    printf '%s\n' "${code:-000}"
}

api_base_url() {
    if [[ -n "${ARMATURE_GITHUB_API+x}" ]]; then
        if [[ -z "$ARMATURE_GITHUB_API" || "$ARMATURE_GITHUB_API" == "0" ]]; then
            return 1
        fi
        printf '%s\n' "${ARMATURE_GITHUB_API%/}"
        return 0
    fi
    if [[ "${GITHUB_RELEASE_BASE%/}" == "https://github.com" ]]; then
        printf '%s\n' "https://api.github.com"
        return 0
    fi
    return 1
}

# Download one exact release asset (tag + filename) from the GitHub API.
# The releases/download gateway on github.com can 504 while this asset URL
# still redirects to the release blob. Checksums are verified by the caller.
api_curl() {
    local dest="$1" url="$2" accept="$3" token="${4:-}"
    if [[ -n "$token" ]]; then
        curl_resilient "$dest" "$url" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            -H "Authorization: Bearer ${token}"
    else
        curl_resilient "$dest" "$url" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -H "X-GitHub-Api-Version: 2022-11-28"
    fi
}

api_download() {
    local filename="$1" dest="$2" api meta url code token
    api=$(api_base_url) || return 1
    if ! command -v python3 >/dev/null 2>&1; then
        echo "WARN: python3 is required for the GitHub asset API; skipping it" >&2
        return 1
    fi
    meta="$WORKDIR/release.json"
    if [[ ! -s "$meta" ]]; then
        token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
        local rel="${api}/repos/${REPO}/releases/tags/${VERSION}"
        code=$(api_curl "$meta" "$rel" "application/vnd.github+json" "$token")
        # A job token is scoped to one repo and can 401/403 on a public
        # tool repo. Retry once with no credential. Do not double-retry 5xx;
        # curl_retry already spent the backoff budget.
        if [[ "$code" == "401" || "$code" == "403" ]]; then
            rm -f "$meta"
            code=$(api_curl "$meta" "$rel" "application/vnd.github+json" "")
        fi
        if [[ "$code" != "200" ]]; then
            rm -f "$meta"
            return 1
        fi
    fi
    url=$(python3 - "$meta" "$filename" "$api" <<'PY'
import json
import sys

meta, name, api = sys.argv[1], sys.argv[2], sys.argv[3]
prefix = api.rstrip("/") + "/"
with open(meta, encoding="utf-8") as fh:
    data = json.load(fh)
for asset in data.get("assets") or []:
    if asset.get("name") != name:
        continue
    url = asset.get("url") or ""
    if url.startswith(prefix):
        print(url)
        raise SystemExit(0)
    raise SystemExit(1)
raise SystemExit(1)
PY
    ) || return 1
    case "$url" in
        "$api"/*) ;;
        *) echo "FAIL: asset URL for $filename is outside $api" >&2; return 1 ;;
    esac
    token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
    if [[ -n "$token" ]]; then
        code=$(curl_resilient "$dest" "$url" \
            -L --max-redirs 5 \
            -H "Accept: application/octet-stream" \
            -H "User-Agent: armature-install-verified-release" \
            -H "Authorization: Bearer ${token}")
        # A scoped job token can 401/403 on a public tool repo.
        if [[ "$code" == "401" || "$code" == "403" ]]; then
            rm -f "$dest"
            code=$(curl_resilient "$dest" "$url" \
                -L --max-redirs 5 \
                -H "Accept: application/octet-stream" \
                -H "User-Agent: armature-install-verified-release")
        fi
    else
        code=$(curl_resilient "$dest" "$url" \
            -L --max-redirs 5 \
            -H "Accept: application/octet-stream" \
            -H "User-Agent: armature-install-verified-release")
    fi
    [[ "$code" == "200" ]]
}

# Prefer the asset API for the pinned tag and filename. Fall back to the
# release download URL for the same name. The tag is the Makefile version,
# never a floating latest tag.
fetch_named() {
    local filename="$1" dest="$2" release_url="$3"
    if api_base_url >/dev/null; then
        if api_download "$filename" "$dest"; then
            echo "fetched $filename via GitHub release asset API"
            return 0
        fi
        echo "WARN: GitHub asset API failed for $filename; trying $release_url" >&2
    fi
    curl_get "$release_url" "$dest"
}

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
            if (NF == 1 && length($1) == 64 && $1 ~ /^[0-9a-fA-F]+$/) { print tolower(h); exit }
            if (n == b) { print tolower(h); exit }
        }
    ' "$sums"
}

path_component_ok() {
    case "$1" in
        ''|*'..'*|*'/'*|*'\\'*) return 1 ;;
    esac
    case "$1" in
        *[!A-Za-z0-9._+-]*) return 1 ;;
    esac
    return 0
}

cache_root() {
    if [[ "${ARMATURE_TOOL_CACHE-}" == "0" ]]; then
        return 1
    fi
    if [[ -n "${ARMATURE_TOOL_CACHE-}" ]]; then
        printf '%s\n' "$ARMATURE_TOOL_CACHE"
        return 0
    fi
    if [[ -n "${XDG_CACHE_HOME-}" ]]; then
        printf '%s\n' "$XDG_CACHE_HOME/armature/github-releases"
        return 0
    fi
    if [[ -z "${HOME-}" ]]; then
        return 1
    fi
    printf '%s\n' "$HOME/.cache/armature/github-releases"
}

# Cache identity is owner/name/version/sha256/tarball.
cache_file_path() {
    local hash="$1" root owner name
    root=$(cache_root) || return 1
    owner="${REPO%%/*}"
    name="${REPO#*/}"
    if [[ "$owner" == "$REPO" || "$name" == *"/"* ]]; then
        return 1
    fi
    path_component_ok "$owner" || return 1
    path_component_ok "$name" || return 1
    path_component_ok "$VERSION" || return 1
    path_component_ok "$TARBALL" || return 1
    path_component_ok "$hash" || return 1
    if [[ ${#hash} -ne 64 ]]; then
        return 1
    fi
    printf '%s\n' "$root/$owner/$name/$VERSION/$hash/$TARBALL"
}

cache_copy_if_valid() {
    local hash="$1" dest="$2" path got
    path=$(cache_file_path "$hash") || return 1
    if [[ ! -f "$path" ]]; then
        return 1
    fi
    got=$(file_sha256 "$path")
    if [[ "$got" != "$hash" ]]; then
        echo "WARN: cached $TARBALL at $hash failed checksum; ignoring" >&2
        rm -f "$path"
        return 1
    fi
    cp "$path" "$dest"
    got=$(file_sha256 "$dest")
    if [[ "$got" != "$hash" ]]; then
        rm -f "$dest"
        return 1
    fi
    return 0
}

store_cache() {
    local src="$1" hash="$2" path dir tmp got
    path=$(cache_file_path "$hash") || return 0
    dir=$(dirname "$path")
    mkdir -p "$dir" || return 0
    tmp=$(mktemp "$dir/.partial.XXXXXX") || return 0
    if ! cp "$src" "$tmp"; then
        rm -f "$tmp"
        return 0
    fi
    got=$(file_sha256 "$tmp")
    if [[ "$got" != "$hash" ]]; then
        rm -f "$tmp"
        return 0
    fi
    mv -f "$tmp" "$path"
}

# When the checksums URL is down, accept a cache entry only if exactly one
# version directory contains bytes that hash to its directory name.
restore_unique_cached() {
    local dest="$1" root owner name verdir base path got n found hash
    root=$(cache_root) || return 1
    owner="${REPO%%/*}"
    name="${REPO#*/}"
    verdir="$root/$owner/$name/$VERSION"
    if [[ ! -d "$verdir" ]]; then
        return 1
    fi
    n=0
    found=""
    hash=""
    for path in "$verdir"/*; do
        [[ -d "$path" ]] || continue
        base=$(basename "$path")
        if [[ ${#base} -ne 64 ]]; then
            continue
        fi
        case "$base" in
            *[!0-9a-f]*) continue ;;
        esac
        if [[ ! -f "$path/$TARBALL" ]]; then
            continue
        fi
        got=$(file_sha256 "$path/$TARBALL")
        if [[ "$got" != "$base" ]]; then
            echo "WARN: cached $TARBALL at $base failed checksum; ignoring" >&2
            continue
        fi
        n=$((n + 1))
        found="$path/$TARBALL"
        hash="$base"
    done
    if [[ "$n" -ne 1 ]]; then
        return 1
    fi
    cp "$found" "$dest"
    got=$(file_sha256 "$dest")
    if [[ "$got" != "$hash" ]]; then
        rm -f "$dest"
        return 1
    fi
    HASH="$hash"
    return 0
}

TARBALL_DEST="$WORKDIR/$TARBALL"
from_cache=0
if fetch_named "$CHECKSUMS" "$WORKDIR/checksums" "$CHECKSUMS_URL"; then
    HASH=$(expected_hash "$WORKDIR/checksums" "$TARBALL")
    if [[ -z "$HASH" || ${#HASH} -ne 64 ]]; then
        echo "FAIL: checksums file has no sha256 entry for $TARBALL" >&2
        echo "checksums file:" >&2
        cat "$WORKDIR/checksums" >&2
        exit 1
    fi
    if cache_copy_if_valid "$HASH" "$TARBALL_DEST"; then
        from_cache=1
        echo "cache hit: $REPO@$VERSION $TARBALL sha256 $HASH"
    elif ! fetch_named "$TARBALL" "$TARBALL_DEST" "$TARBALL_URL"; then
        echo "FAIL: download failed for $TARBALL_URL" >&2
        exit 1
    fi
elif restore_unique_cached "$TARBALL_DEST"; then
    from_cache=1
    echo "checksums endpoint unavailable; using cached $TARBALL sha256 $HASH"
else
    echo "FAIL: download failed for $CHECKSUMS_URL and no single verified cached artifact for $REPO@$VERSION" >&2
    exit 1
fi

GOT=$(file_sha256 "$TARBALL_DEST")
if [[ "$GOT" != "$HASH" ]]; then
    echo "FAIL: checksum mismatch for $TARBALL" >&2
    echo "  expected: $HASH" >&2
    echo "  actual:   $GOT" >&2
    exit 1
fi
if [[ "$from_cache" -eq 0 ]]; then
    echo "OK: $TARBALL sha256 matches $CHECKSUMS"
else
    echo "OK: $TARBALL sha256 matches cached $HASH"
fi
store_cache "$TARBALL_DEST" "$HASH" || echo "WARN: could not update tool cache for $TARBALL" >&2

mkdir -p "$WORKDIR/extract"
tar -xzf "$TARBALL_DEST" -C "$WORKDIR/extract"

FOUND=$(find "$WORKDIR/extract" -type f -name "$BIN" | head -n 1)
if [[ -z "$FOUND" ]]; then
    echo "FAIL: tarball $TARBALL does not contain binary $BIN" >&2
    find "$WORKDIR/extract" -type f >&2
    exit 1
fi

mkdir -p "$DEST"
install -m 0755 "$FOUND" "$DEST/$BIN"
echo "Installed $DEST/$BIN ($BIN@$VERSION)"
