#!/usr/bin/env bash
# Install one pinned GitHub release binary.
# The only trusted digest is scripts/github-release.sha256 (override with
# ARMATURE_TOOL_PIN_FILE). A downloaded checksum file, a cache path, and a
# filename are not digests.
#
# TOOL_FETCH_RETRY_MAX_TIME / ARMATURE_TOOL_FETCH_RETRY_MAX_TIME is the
# wall-clock budget for one artifact: every attempt, redirect, and the
# asset-API then release-URL fallback. It is not a per-curl timeout.
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/install-verified-github-release.sh --repo OWNER/NAME --version VER \
  --tarball NAME --bin NAME --dest DIR

Tarball names may contain {version} {ver} {os} {arch}.
The expected sha256 is looked up from scripts/github-release.sha256.

Optional:
  TARBALL_URL                  override the release download URL (tests)
  GITHUB_RELEASE_BASE          default https://github.com
  ARMATURE_GITHUB_API          asset API base, or 0 to skip; default
                               https://api.github.com when the release base
                               is https://github.com
  ARMATURE_TOOL_PIN_FILE       pin table (tests)
  ARMATURE_TOOL_CACHE          cache directory, or 0 to disable
  ARMATURE_TOOL_CACERT         extra CA bundle passed to curl (tests)
  ARMATURE_TOOL_FETCH_RETRIES  extra attempts per URL after the first (default 5, max 8)
  ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
                               wall-clock seconds for this artifact (default 120, max 300)
  ARMATURE_TOOL_FETCH_BACKOFF_SECS
                               base sleep between attempts (default 1, max 30)
EOF
    exit 2
}

REPO=""
VERSION=""
TARBALL_TMPL=""
BIN=""
DEST=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo) REPO="${2:-}"; shift 2 ;;
        --version) VERSION="${2:-}"; shift 2 ;;
        --tarball) TARBALL_TMPL="${2:-}"; shift 2 ;;
        --bin) BIN="${2:-}"; shift 2 ;;
        --dest) DEST="${2:-}"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "FAIL: unknown argument: $1" >&2; usage ;;
    esac
done

if [[ -z "$REPO" || -z "$VERSION" || -z "$TARBALL_TMPL" || -z "$BIN" || -z "$DEST" ]]; then
    echo "FAIL: --repo, --version, --tarball, --bin, and --dest are required" >&2
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
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PIN_FILE="${ARMATURE_TOOL_PIN_FILE:-$SCRIPT_DIR/github-release.sha256}"
GITHUB_RELEASE_BASE="${GITHUB_RELEASE_BASE:-https://github.com}"
TARBALL_URL="${TARBALL_URL:-${GITHUB_RELEASE_BASE%/}/${REPO}/releases/download/${VERSION}/${TARBALL}}"

if ! command -v curl >/dev/null 2>&1; then
    echo "FAIL: curl is required to install $BIN" >&2
    exit 1
fi
if [[ ! -f "$PIN_FILE" ]]; then
    echo "FAIL: pin table not found: $PIN_FILE" >&2
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

FETCH_RETRIES=$(require_uint ARMATURE_TOOL_FETCH_RETRIES "${ARMATURE_TOOL_FETCH_RETRIES:-5}" 0 8)
FETCH_RETRY_MAX_TIME=$(require_uint ARMATURE_TOOL_FETCH_RETRY_MAX_TIME "${ARMATURE_TOOL_FETCH_RETRY_MAX_TIME:-120}" 1 300)
FETCH_BACKOFF=$(require_uint ARMATURE_TOOL_FETCH_BACKOFF_SECS "${ARMATURE_TOOL_FETCH_BACKOFF_SECS:-1}" 0 30)

lookup_pin() {
    local hash file repo ver
    PIN=""
    while read -r hash file repo ver _; do
        case "$hash" in
            ''|\#*) continue ;;
        esac
        if [[ "$repo" == "$REPO" && "$ver" == "$VERSION" && "$file" == "$TARBALL" ]]; then
            PIN=$(printf '%s' "$hash" | tr '[:upper:]' '[:lower:]')
        fi
    done < "$PIN_FILE"
    if [[ ${#PIN} -ne 64 ]] || [[ ! "$PIN" =~ ^[0-9a-f]+$ ]]; then
        echo "FAIL: no pinned sha256 for $REPO $VERSION $TARBALL in $PIN_FILE" >&2
        exit 1
    fi
}

lookup_pin

file_sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

url_host() {
    local u="$1"
    u="${u#*://}"
    u="${u%%/*}"
    u="${u%%\?*}"
    u="${u#*@}"
    u="${u%%:*}"
    printf '%s' "$u" | tr '[:upper:]' '[:lower:]'
}

cache_file() {
    local root owner name
    if [[ "${ARMATURE_TOOL_CACHE-}" == "0" ]]; then
        return 1
    fi
    if [[ -n "${ARMATURE_TOOL_CACHE-}" ]]; then
        root="$ARMATURE_TOOL_CACHE"
    elif [[ -n "${XDG_CACHE_HOME-}" ]]; then
        root="$XDG_CACHE_HOME/armature/github-releases"
    elif [[ -n "${HOME-}" ]]; then
        root="$HOME/.cache/armature/github-releases"
    else
        return 1
    fi
    owner="${REPO%%/*}"
    name="${REPO#*/}"
    case "$owner$name$VERSION$TARBALL$PIN" in
        *..*|*' '*|*'\\'*) return 1 ;;
    esac
    if [[ "$owner" == "$REPO" || "$name" == */* ]]; then
        return 1
    fi
    printf '%s\n' "$root/$owner/$name/$VERSION/$PIN/$TARBALL"
}

# One HTTP transaction. Prints "<curl-exit> <http-code>" on stdout.
# Headers land in $WORKDIR/last.hdr. Redirects are not followed here.
# A non-zero curl exit (including a truncated body) is a failed transfer
# even when the status line was 200.
http_once() {
    local dest="$1" url="$2" token="$3" limit="$4" accept="${5:-}" st code
    if [[ -z "$accept" ]]; then
        accept="*/*"
    fi
    LAST_HDR="$WORKDIR/last.hdr"
    set +e
    if [[ -n "${ARMATURE_TOOL_CACERT:-}" && -n "$token" ]]; then
        code=$(curl -sS --max-time "$limit" --cacert "$ARMATURE_TOOL_CACERT" \
            -H "Authorization: Bearer ${token}" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -D "$LAST_HDR" -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    elif [[ -n "${ARMATURE_TOOL_CACERT:-}" ]]; then
        code=$(curl -sS --max-time "$limit" --cacert "$ARMATURE_TOOL_CACERT" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -D "$LAST_HDR" -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    elif [[ -n "$token" ]]; then
        code=$(curl -sS --max-time "$limit" \
            -H "Authorization: Bearer ${token}" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -D "$LAST_HDR" -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    else
        code=$(curl -sS --max-time "$limit" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -D "$LAST_HDR" -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    fi
    set -e
    printf '%s %s\n' "$st" "${code:-000}"
}

time_left() {
    local used=$((SECONDS - FETCH_START))
    local left=$((FETCH_RETRY_MAX_TIME - used))
    if [[ "$left" -lt 1 ]]; then
        printf '0\n'
    else
        printf '%s\n' "$left"
    fi
}

# Follow at most 5 redirects. Only https targets. Drop Authorization when
# the next host differs from the URL we were asked to fetch.
fetch_url() {
    local url="$1" dest="$2" token="$3" accept="${4:-}"
    local hops=0 cur="$url" origin host loc st code left
    origin=$(url_host "$url")
    while [[ "$hops" -le 5 ]]; do
        left=$(time_left)
        if [[ "$left" -eq 0 ]]; then
            echo "FAIL: tool fetch exceeded ${FETCH_RETRY_MAX_TIME}s wall-clock budget" >&2
            return 1
        fi
        host=$(url_host "$cur")
        if [[ "$host" != "$origin" ]]; then
            token=""
        fi
        http_once "$dest" "$cur" "$token" "$left" "$accept" > "$WORKDIR/status"
        read -r st code < "$WORKDIR/status"
        LAST_HTTP="$code"
        if [[ "$st" -ne 0 ]]; then
            rm -f "$dest"
            return 1
        fi
        case "$code" in
            200)
                if [[ ! -s "$dest" ]]; then
                    return 1
                fi
                return 0
                ;;
            301|302|303|307|308)
                loc=$(awk '
                    {
                        line = $0
                        gsub(/\r/, "", line)
                        low = tolower(line)
                        if (low ~ /^location:[ \t]*/) {
                            sub(/^[^:]+:[ \t]*/, "", line)
                            print line
                        }
                    }
                ' "$LAST_HDR" | tail -n 1)
                case "$loc" in
                    https://*) ;;
                    *)
                        echo "FAIL: redirect from $cur is not https ($loc)" >&2
                        rm -f "$dest"
                        return 2
                        ;;
                esac
                cur="$loc"
                hops=$((hops + 1))
                ;;
            *)
                rm -f "$dest"
                return 1
                ;;
        esac
    done
    echo "FAIL: too many redirects for $url" >&2
    return 2
}

digest_matches() {
    local got
    got=$(file_sha256 "$1")
    [[ "$got" == "$PIN" ]]
}

# Retry transport failures inside the wall-clock budget. A finished body
# whose digest is not the pin is not retried; the caller tries the other URL.
fetch_matching() {
    local url="$1" token="$2" dest="$3" accept="${4:-}"
    local attempt=0 st
    while [[ "$attempt" -le "$FETCH_RETRIES" ]]; do
        if [[ "$(time_left)" -eq 0 ]]; then
            return 1
        fi
        set +e
        fetch_url "$url" "$dest" "$token" "$accept"
        st=$?
        set -e
        if [[ "$st" -eq 2 ]]; then
            return 1
        fi
        if [[ "$st" -eq 0 ]]; then
            if digest_matches "$dest"; then
                return 0
            fi
            SAW_MISMATCH=1
            echo "WARN: $url bytes do not match pinned sha256" >&2
            rm -f "$dest"
            return 1
        fi
        rm -f "$dest"
        attempt=$((attempt + 1))
        if [[ "$attempt" -le "$FETCH_RETRIES" && "$FETCH_BACKOFF" -gt 0 && "$(time_left)" -gt 0 ]]; then
            sleep $((FETCH_BACKOFF * attempt))
        fi
    done
    return 1
}

# Same budget as fetch_matching, without a digest check (release JSON).
fetch_retry() {
    local url="$1" dest="$2" token="$3" accept="$4"
    local attempt=0 st
    while [[ "$attempt" -le "$FETCH_RETRIES" ]]; do
        if [[ "$(time_left)" -eq 0 ]]; then
            return 1
        fi
        set +e
        fetch_url "$url" "$dest" "$token" "$accept"
        st=$?
        set -e
        if [[ "$st" -eq 0 ]]; then
            return 0
        fi
        if [[ "$st" -eq 2 ]]; then
            return 1
        fi
        attempt=$((attempt + 1))
        if [[ "$attempt" -le "$FETCH_RETRIES" && "$FETCH_BACKOFF" -gt 0 && "$(time_left)" -gt 0 ]]; then
            sleep $((FETCH_BACKOFF * attempt))
        fi
    done
    return 1
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

api_asset_url() {
    local api="$1" meta="$2"
    command -v python3 >/dev/null 2>&1 || return 1
    python3 - "$meta" "$TARBALL" "$api" <<'PY'
import json, sys
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
}

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
FETCH_START=$SECONDS
SAW_MISMATCH=0
LAST_HTTP=""
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
TARBALL_DEST="$WORKDIR/$TARBALL"

echo "Installing $BIN@$VERSION (pinned sha256 ${PIN})"
echo "  budget: ${FETCH_RETRY_MAX_TIME}s wall-clock for this artifact"

from_cache=0
CACHE_PATH=""
if CACHE_PATH=$(cache_file); then
    if [[ -f "$CACHE_PATH" ]] && digest_matches "$CACHE_PATH"; then
        cp "$CACHE_PATH" "$TARBALL_DEST"
        from_cache=1
        echo "cache hit: pinned sha256 $PIN"
    elif [[ -f "$CACHE_PATH" ]]; then
        echo "WARN: cached $TARBALL does not match pinned sha256; ignoring" >&2
        rm -f "$CACHE_PATH"
    fi
fi

downloaded=0
if [[ "$from_cache" -eq 0 ]]; then
    api=""
    if api=$(api_base_url); then
        meta="$WORKDIR/release.json"
        rel="${api}/repos/${REPO}/releases/tags/${VERSION}"
        api_meta_ok=0
        if fetch_retry "$rel" "$meta" "$TOKEN" "application/vnd.github+json"; then
            api_meta_ok=1
        elif [[ -n "$TOKEN" && ( "$LAST_HTTP" == "401" || "$LAST_HTTP" == "403" ) ]]; then
            # A job token is often refused on another public repo.
            TOKEN=""
            if fetch_retry "$rel" "$meta" "" "application/vnd.github+json"; then
                api_meta_ok=1
            fi
        fi
        if [[ "$api_meta_ok" -eq 1 ]]; then
            asset=$(api_asset_url "$api" "$meta" || true)
            case "$asset" in
                "$api"/*)
                    if fetch_matching "$asset" "$TOKEN" "$TARBALL_DEST" "application/octet-stream"; then
                        downloaded=1
                        echo "fetched $TARBALL via GitHub release asset API"
                    fi
                    ;;
            esac
        else
            echo "WARN: GitHub asset API failed for $REPO@$VERSION (HTTP ${LAST_HTTP:-none})" >&2
        fi
    fi
    if [[ "$downloaded" -eq 0 ]]; then
        if fetch_matching "$TARBALL_URL" "$TOKEN" "$TARBALL_DEST"; then
            downloaded=1
            echo "fetched $TARBALL via release download URL"
        fi
    fi
fi

if [[ "$from_cache" -eq 0 && "$downloaded" -eq 0 ]]; then
    if [[ "$SAW_MISMATCH" -eq 1 && -f "$TARBALL_DEST" ]]; then
        got=$(file_sha256 "$TARBALL_DEST")
        echo "FAIL: checksum mismatch for $TARBALL" >&2
        echo "  expected: $PIN" >&2
        echo "  actual:   $got" >&2
    elif [[ "$SAW_MISMATCH" -eq 1 ]]; then
        echo "FAIL: checksum mismatch for $TARBALL" >&2
        echo "  expected: $PIN" >&2
        echo "  actual:   (response did not match the pinned sha256)" >&2
    else
        echo "FAIL: could not fetch $TARBALL matching pinned sha256 $PIN" >&2
    fi
    exit 1
fi
if ! digest_matches "$TARBALL_DEST"; then
    got=$(file_sha256 "$TARBALL_DEST")
    echo "FAIL: checksum mismatch for $TARBALL" >&2
    echo "  expected: $PIN" >&2
    echo "  actual:   $got" >&2
    exit 1
fi
echo "OK: $TARBALL sha256 matches pinned $PIN"

if [[ -n "$CACHE_PATH" && "$from_cache" -eq 0 ]]; then
    mkdir -p "$(dirname "$CACHE_PATH")"
    cp "$TARBALL_DEST" "$CACHE_PATH.tmp"
    if digest_matches "$CACHE_PATH.tmp"; then
        mv -f "$CACHE_PATH.tmp" "$CACHE_PATH"
    else
        rm -f "$CACHE_PATH.tmp"
    fi
fi

mkdir -p "$WORKDIR/extract"
tar -xzf "$TARBALL_DEST" -C "$WORKDIR/extract"
FOUND=$(find "$WORKDIR/extract" -type f -name "$BIN" | head -n 1)
if [[ -z "$FOUND" ]]; then
    echo "FAIL: tarball $TARBALL does not contain binary $BIN" >&2
    exit 1
fi
mkdir -p "$DEST"
install -m 0755 "$FOUND" "$DEST/$BIN"
echo "Installed $DEST/$BIN ($BIN@$VERSION)"
