#!/usr/bin/env bash
# Install pinned GNU Make for the Windows CI job.
#
# The windows-latest image already has make.exe, but that file is MinGW
# mingw32-make. This script installs the ezwinports GNU Make 4.4.1 binary
# (the one Chocolatey package make 4.4.1 embeds) and puts it first on PATH.
#
# Sources and checksums live in scripts/windows-make.sha256. Each source is
# fetched with a bounded retry. A checksum mismatch does not retry that
# source. The next source is tried when the current one is exhausted, so one
# host being down does not fail the job. make.exe is hashed again before it
# is published. When every source fails, the script exits non-zero.
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
PIN_FILE=${MAKE_PIN_FILE:-$SCRIPT_DIR/windows-make.sha256}
CURL=${MAKE_CURL:-curl}
ATTEMPTS=${MAKE_FETCH_ATTEMPTS:-4}
BACKOFF=${MAKE_FETCH_BACKOFF_SECS:-2}

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

require_uint() {
    local name="$1" value="$2" min="$3" max="$4" n
    case "$value" in
        ''|*[!0-9]*) fail "$name must be an integer (got $value)" ;;
    esac
    n=$((10#$value))
    if [[ "$n" -lt "$min" || "$n" -gt "$max" ]]; then
        fail "$name must be between $min and $max (got $value)"
    fi
    printf '%s\n' "$n"
}

ATTEMPTS=$(require_uint MAKE_FETCH_ATTEMPTS "$ATTEMPTS" 1 8)
BACKOFF=$(require_uint MAKE_FETCH_BACKOFF_SECS "$BACKOFF" 0 30)

if [[ -n "${MAKE_INSTALL_ROOT:-}" ]]; then
    ROOT=$MAKE_INSTALL_ROOT
else
    base=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
    if command -v cygpath >/dev/null 2>&1; then
        base=$(cygpath -u "$base")
    fi
    ROOT=$base/gnumake-4.4.1
fi
case "$ROOT" in
    *gnumake-4.4.1) ;;
    *) fail "install root must contain gnumake-4.4.1 (got $ROOT)" ;;
esac

command -v "$CURL" >/dev/null 2>&1 || fail "curl is required ($CURL)"
command -v unzip >/dev/null 2>&1 || fail "unzip is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"
[[ -f "$PIN_FILE" ]] || fail "pin file not found: $PIN_FILE"

is_sha256() {
    local s="$1"
    # File digests are SHA-256. Do not spell that as a hex-width regex;
    # that shape is reserved for git object ids.
    [[ ${#s} -eq 64 ]] || return 1
    case "$s" in
        *[!0-9a-f]*) return 1 ;;
    esac
    return 0
}

EXE_SHA=""
VERSION=""
SOURCES=$(mktemp)
WORKDIR=$(mktemp -d)
cleanup() { rm -rf "$WORKDIR" "$SOURCES"; }
trap cleanup EXIT

while IFS= read -r line || [[ -n "$line" ]]; do
    line=${line%$'\r'}
    case "$line" in
        '') continue ;;
        '# exe-sha256: '*) EXE_SHA=${line#'# exe-sha256: '} ;;
        '# version: '*) VERSION=${line#'# version: '} ;;
        '#'*) continue ;;
        *) printf '%s\n' "$line" >> "$SOURCES" ;;
    esac
done < "$PIN_FILE"

is_sha256 "$EXE_SHA" || fail "pin file missing a 64-hex exe-sha256"
case "$VERSION" in
    '') fail "pin file missing version" ;;
    *[!0-9.]*) fail "pin file version is not a dotted number: $VERSION" ;;
esac

source_count=$(grep -c . "$SOURCES" || true)
if [[ "$source_count" -lt 1 || "$source_count" -gt 4 ]]; then
    fail "pin file must list 1 to 4 sources (got ${source_count:-0})"
fi

sha_file() {
    sha256sum "$1" | awk 'NR==1 { print $1 }'
}

publish() {
    local dir="$1" out
    out=$dir
    if command -v cygpath >/dev/null 2>&1; then
        out=$(cygpath -w "$dir")
    fi
    if [[ -n "${GITHUB_PATH:-}" ]]; then
        printf '%s\n' "$out" >> "$GITHUB_PATH"
    fi
    PATH="${dir}:${PATH}"
    export PATH
    echo "make ${VERSION}: installed ${out}/make.exe"
}

install_verified_exe() {
    local archive="$1" inner="$2" extract candidate got ver exe
    extract="$WORKDIR/extract"
    rm -rf "$extract"
    mkdir -p "$extract"
    unzip -q -o "$archive" -d "$extract"
    candidate="$extract/$inner"
    if [[ ! -f "$candidate" ]]; then
        echo "make ${VERSION}: archive has no ${inner}; skipping this source" >&2
        return 1
    fi
    got=$(sha_file "$candidate")
    if [[ "$got" != "$EXE_SHA" ]]; then
        echo "make ${VERSION}: make.exe checksum mismatch (got ${got} want ${EXE_SHA}); skipping this source" >&2
        return 1
    fi
    rm -rf "$ROOT"
    mkdir -p "$ROOT/bin"
    cp "$candidate" "$ROOT/bin/make.exe"
    chmod +x "$ROOT/bin/make.exe"
    exe="$ROOT/bin/make.exe"
    if ! ver=$("$exe" --version | tr -d '\r'); then
        rm -rf "$ROOT"
        fail "pinned make.exe exited non-zero for --version"
    fi
    ver=${ver%%$'\n'*}
    if [[ "$ver" != "GNU Make ${VERSION}" ]]; then
        rm -rf "$ROOT"
        fail "pinned make.exe reported '${ver}', want 'GNU Make ${VERSION}'"
    fi
    publish "$ROOT/bin"
    echo "make ${VERSION}: version output follows"
    "$exe" --version | tr -d '\r'
    return 0
}

try_source() {
    local want="$1" url="$2" inner="$3" attempt delay archive got
    case "$url" in
        https://*) ;;
        *)
            echo "make ${VERSION}: refusing non-https source ${url}" >&2
            return 1
            ;;
    esac
    case "$inner" in
        /*|*..*|*' '*)
            echo "make ${VERSION}: refusing inner path ${inner}" >&2
            return 1
            ;;
    esac
    attempt=1
    delay=$BACKOFF
    archive="$WORKDIR/archive"
    while [[ "$attempt" -le "$ATTEMPTS" ]]; do
        echo "make ${VERSION}: attempt ${attempt}/${ATTEMPTS} ${url}"
        rm -f "$archive"
        if "$CURL" -fsSL \
            --connect-timeout 15 \
            --max-time 60 \
            --retry 0 \
            --max-redirs 5 \
            --proto-redir =https \
            -L \
            -A armature-install-windows-make \
            -o "$archive" \
            "$url"
        then
            got=$(sha_file "$archive")
            if [[ "$got" != "$want" ]]; then
                echo "make ${VERSION}: checksum mismatch for ${url}: got ${got} want ${want} (not retrying this source)" >&2
                rm -f "$archive"
                return 1
            fi
            if install_verified_exe "$archive" "$inner"; then
                return 0
            fi
            return 1
        fi
        rm -f "$archive"
        echo "make ${VERSION}: fetch failed for ${url} (attempt ${attempt}/${ATTEMPTS})" >&2
        if [[ "$attempt" -lt "$ATTEMPTS" ]]; then
            echo "make ${VERSION}: retrying in ${delay}s" >&2
            sleep "$delay"
            delay=$((delay * 2))
        fi
        attempt=$((attempt + 1))
    done
    echo "make ${VERSION}: exhausted ${ATTEMPTS} attempts for ${url}" >&2
    return 1
}

installed=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    want=${line%%  *}
    rest=${line#*  }
    url=${rest%%  *}
    inner=${rest#*  }
    if [[ "$want" == "$line" || "$url" == "$rest" || -z "$inner" || "$inner" == "$rest" ]]; then
        fail "malformed pin line: $line"
    fi
    is_sha256 "$want" || fail "archive sha256 is not 64 lowercase hex: $want"
    if try_source "$want" "$url" "$inner"; then
        installed=1
        break
    fi
done < "$SOURCES"

if [[ "$installed" -ne 1 ]]; then
    fail "GNU Make ${VERSION} was not installed. Every pinned source exhausted its retries or failed verification. Refusing to continue without make."
fi
