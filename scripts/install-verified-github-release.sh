#!/usr/bin/env bash
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
  ARMATURE_TOOL_CACERT         extra CA bundle passed to curl (tests)
  ARMATURE_TOOL_FETCH_RETRIES  extra attempts on the release URL after the first
                               (default 30, max 40). The asset API uses at most
                               two of these so a dead API falls through.
  ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
                               wall-clock seconds for this artifact (default 120, max 300)
  ARMATURE_TOOL_FETCH_BACKOFF_SECS
                               base sleep between attempts (default 1, max 30)
  ARMATURE_TOOL_FETCH_API_MAX_TIME
                               per-curl seconds for the asset API (default 15, max 60)
  ARMATURE_TOOL_FETCH_ATTEMPT_MAX_TIME
                               per-curl seconds for a tarball (default 60, max 120)
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

FETCH_RETRIES=$(require_uint ARMATURE_TOOL_FETCH_RETRIES "${ARMATURE_TOOL_FETCH_RETRIES:-30}" 0 40)
FETCH_RETRY_MAX_TIME=$(require_uint ARMATURE_TOOL_FETCH_RETRY_MAX_TIME "${ARMATURE_TOOL_FETCH_RETRY_MAX_TIME:-120}" 1 300)
FETCH_BACKOFF=$(require_uint ARMATURE_TOOL_FETCH_BACKOFF_SECS "${ARMATURE_TOOL_FETCH_BACKOFF_SECS:-1}" 0 30)
API_ATTEMPT_MAX=$(require_uint ARMATURE_TOOL_FETCH_API_MAX_TIME "${ARMATURE_TOOL_FETCH_API_MAX_TIME:-15}" 1 60)
ATTEMPT_MAX=$(require_uint ARMATURE_TOOL_FETCH_ATTEMPT_MAX_TIME "${ARMATURE_TOOL_FETCH_ATTEMPT_MAX_TIME:-60}" 1 120)
API_EXTRA_MAX=2

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

attempt_limit() {
    local cap="$1" left
    left=$(time_left)
    if [[ "$left" -le 0 ]]; then
        printf '0\n'
        return 0
    fi
    if [[ "$cap" -gt 0 && "$left" -gt "$cap" ]]; then
        printf '%s\n' "$cap"
        return 0
    fi
    printf '%s\n' "$left"
}

# A non-zero curl exit (truncated body, refused redirect) is a failed
# transfer even when a status line was 200.
transfer() {
    local dest="$1" url="$2" limit="$3" accept="${4:-}" st code hdr
    # --max-time 0 means no limit.
    LAST_HTTP=""
    LAST_RETRY_AFTER=""
    if [[ -z "$limit" || "$limit" -le 0 ]]; then
        echo "FAIL: refusing download with a non-positive deadline" >&2
        return 1
    fi
    if [[ -z "$accept" ]]; then
        accept="*/*"
    fi
    hdr="${dest}.hdr"
    rm -f "$hdr"
    set +e
    if [[ -n "${ARMATURE_TOOL_CACERT:-}" ]]; then
        code=$(curl -sS --max-time "$limit" --cacert "$ARMATURE_TOOL_CACERT" \
            -L --max-redirs 5 --proto-redir =https \
            -D "$hdr" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    else
        code=$(curl -sS --max-time "$limit" \
            -L --max-redirs 5 --proto-redir =https \
            -D "$hdr" \
            -H "Accept: ${accept}" \
            -H "User-Agent: armature-install-verified-release" \
            -o "$dest" -w '%{http_code}' \
            "$url")
        st=$?
    fi
    set -e
    LAST_HTTP="$code"
    if [[ -f "$hdr" ]]; then
        LAST_RETRY_AFTER=$(tr -d '\r' < "$hdr" | awk 'tolower($1)=="retry-after:" {print $2; exit}')
        rm -f "$hdr"
    fi
    if [[ "$st" -ne 0 || "$code" != "200" || ! -s "$dest" ]]; then
        rm -f "$dest"
        return 1
    fi
    return 0
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

pause_for_retry() {
    local want="$1" left
    [[ "$want" -gt 0 ]] || return 0
    left=$(time_left)
    [[ "$left" -gt 0 ]] || return 0
    if [[ "$want" -gt "$left" ]]; then
        want="$left"
    fi
    sleep "$want"
}

sleep_before_retry() {
    local attempt="$1" want
    want=$((FETCH_BACKOFF * attempt))
    if [[ "$want" -gt 15 ]]; then
        want=15
    fi
    if [[ "${LAST_RETRY_AFTER:-}" =~ ^[0-9]+$ ]] && [[ "$LAST_RETRY_AFTER" -gt "$want" ]]; then
        want=$LAST_RETRY_AFTER
    fi
    pause_for_retry "$want"
}

digest_matches() {
    local got
    got=$(file_sha256 "$1")
    [[ "$got" == "$PIN" ]]
}

fetch_matching() {
    local url="$1" dest="$2" accept="${3:-}" retries="$4" cap="$5"
    local attempt=0 limit
    while [[ "$attempt" -le "$retries" ]]; do
        limit=$(attempt_limit "$cap")
        if [[ "$limit" -le 0 ]]; then
            return 1
        fi
        if transfer "$dest" "$url" "$limit" "$accept" && digest_matches "$dest"; then
            return 0
        fi
        if [[ -s "$dest" ]]; then
            SAW_MISMATCH=1
            echo "WARN: downloaded bytes do not match pinned sha256" >&2
            rm -f "$dest"
            return 1
        fi
        rm -f "$dest"
        attempt=$((attempt + 1))
        if [[ "$attempt" -le "$retries" ]]; then
            echo "WARN: tarball fetch failed (http ${LAST_HTTP:-none}), retrying" >&2
            sleep_before_retry "$attempt"
        fi
    done
    return 1
}

fetch_retry() {
    local url="$1" dest="$2" accept="$3" retries="$4" cap="$5"
    local attempt=0 limit
    while [[ "$attempt" -le "$retries" ]]; do
        limit=$(attempt_limit "$cap")
        if [[ "$limit" -le 0 ]]; then
            return 1
        fi
        if transfer "$dest" "$url" "$limit" "$accept"; then
            return 0
        fi
        attempt=$((attempt + 1))
        if [[ "$attempt" -le "$retries" ]]; then
            echo "WARN: asset API fetch failed (http ${LAST_HTTP:-none}), retrying" >&2
            sleep_before_retry "$attempt"
        fi
    done
    return 1
}

api_extra_attempts() {
    local n=$FETCH_RETRIES
    if [[ "$n" -gt "$API_EXTRA_MAX" ]]; then
        n=$API_EXTRA_MAX
    fi
    printf '%s\n' "$n"
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
TARBALL_DEST="$WORKDIR/$TARBALL"

echo "Installing $BIN@$VERSION (pinned sha256 ${PIN})"
echo "  budget: ${FETCH_RETRY_MAX_TIME}s wall-clock for this artifact"

downloaded=0
api=""
api_retries=$(api_extra_attempts)
if api=$(api_base_url); then
    meta="$WORKDIR/release.json"
    rel="${api}/repos/${REPO}/releases/tags/${VERSION}"
    if fetch_retry "$rel" "$meta" "application/vnd.github+json" "$api_retries" "$API_ATTEMPT_MAX"; then
        asset=$(api_asset_url "$api" "$meta" || true)
        case "$asset" in
            "$api"/*)
                if [[ "$SAW_MISMATCH" -eq 0 ]] && fetch_matching "$asset" "$TARBALL_DEST" "application/octet-stream" "$api_retries" "$ATTEMPT_MAX"; then
                    downloaded=1
                    echo "fetched $TARBALL via GitHub release asset API"
                fi
                ;;
        esac
    else
        echo "WARN: GitHub asset API failed for $REPO@$VERSION (http ${LAST_HTTP:-none})" >&2
    fi
fi
if [[ "$downloaded" -eq 0 && "$SAW_MISMATCH" -eq 0 ]]; then
    if fetch_matching "$TARBALL_URL" "$TARBALL_DEST" "*/*" "$FETCH_RETRIES" "$ATTEMPT_MAX"; then
        downloaded=1
        echo "fetched $TARBALL via release download URL"
    fi
fi

if [[ "$downloaded" -eq 0 ]]; then
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
        echo "FAIL: could not fetch $TARBALL matching pinned sha256 $PIN (last http ${LAST_HTTP:-none})" >&2
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
