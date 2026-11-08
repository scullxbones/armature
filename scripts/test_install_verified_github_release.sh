#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/install-verified-github-release.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if [[ ! -x "$SCRIPT" ]]; then
    echo "FAIL: $SCRIPT is not executable"
    exit 1
fi

mkdir -p "$WORKDIR/bin" "$WORKDIR/www/example/tool/releases/download/v9.9.9"
cat > "$WORKDIR/bin/tool" <<'EOF'
#!/bin/sh
echo fake-tool
EOF
chmod +x "$WORKDIR/bin/tool"

TARBALL="tool-9.9.9-linux-amd64.tar.gz"
REL="example/tool/releases/download/v9.9.9"
tar -C "$WORKDIR/bin" -czf "$WORKDIR/www/$REL/$TARBALL" tool
HASH=$(sha256sum "$WORKDIR/www/$REL/$TARBALL" | awk '{print $1}')
write_good_sums() {
    printf '%s  %s\n' "$HASH" "$TARBALL" > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"
}
write_good_sums

MODE_FILE="$WORKDIR/mode"
COUNTS_FILE="$WORKDIR/counts"
PORT_FILE="$WORKDIR/port"
printf 'ok\n' > "$MODE_FILE"
: > "$COUNTS_FILE"

python3 - "$WORKDIR/www" "$PORT_FILE" "$MODE_FILE" "$COUNTS_FILE" <<'PY' &
import http.server
import os
import sys
from pathlib import Path

root = Path(sys.argv[1])
port_file = Path(sys.argv[2])
mode_file = Path(sys.argv[3])
counts_file = Path(sys.argv[4])
os.chdir(root)

class Handler(http.server.SimpleHTTPRequestHandler):
    def serve_bytes(self, status, body, content_type):
        data = body if isinstance(body, bytes) else body.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def serve_api(self, path):
        import json
        host, port = self.server.server_address
        base = f"http://{host}:{port}/api"
        if path == "/api/repos/example/tool/releases/tags/v9.9.9":
            body = json.dumps({
                "assets": [
                    {
                        "name": "tool-9.9.9-checksums.txt",
                        "url": base + "/asset/checksums",
                    },
                    {
                        "name": "tool-9.9.9-linux-amd64.tar.gz",
                        "url": base + "/asset/tarball",
                    },
                ]
            })
            self.serve_bytes(200, body, "application/json")
            return
        rel = Path("example/tool/releases/download/v9.9.9")
        if path == "/api/asset/checksums":
            self.serve_bytes(200, (rel / "tool-9.9.9-checksums.txt").read_bytes(), "text/plain")
            return
        if path == "/api/asset/tarball":
            mode = mode_file.read_text(encoding="utf-8").strip() if mode_file.exists() else "ok"
            if mode == "api-403-once":
                hits = 0
                if counts_file.exists():
                    hits = sum(
                        1
                        for line in counts_file.read_text(encoding="utf-8").splitlines()
                        if line.split("?", 1)[0] == path
                    )
                if hits == 1:
                    self.serve_bytes(403, "forbidden", "text/plain")
                    return
            self.serve_bytes(200, (rel / "tool-9.9.9-linux-amd64.tar.gz").read_bytes(), "application/gzip")
            return
        self.serve_bytes(404, "missing api route", "text/plain")

    def do_GET(self):
        mode = mode_file.read_text(encoding="utf-8").strip() if mode_file.exists() else "ok"
        prior = 0
        if counts_file.exists():
            prior = sum(1 for line in counts_file.read_text(encoding="utf-8").splitlines() if line == self.path)
        with counts_file.open("a", encoding="utf-8") as fh:
            fh.write(self.path + "\n")
        n = prior + 1
        path = self.path.split("?", 1)[0]
        if path.startswith("/api/"):
            self.serve_api(path)
            return
        fail = False
        if mode == "always-504" or mode == "release-504" or mode == "api-403-once":
            fail = True
        elif mode == "flaky" and n == 1:
            fail = True
        elif mode == "tarball-504" and path.endswith(".tar.gz"):
            fail = True
        if fail:
            body = b"gateway timeout"
            self.send_response(504)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        return super().do_GET()

    def log_message(self, fmt, *args):
        pass

httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
port_file.write_text(str(httpd.server_address[1]), encoding="utf-8")
httpd.serve_forever()
PY
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true; rm -rf "$WORKDIR"' EXIT

for _ in $(seq 1 50); do
    if [[ -s "$PORT_FILE" ]]; then
        break
    fi
    sleep 0.05
done
PORT=$(cat "$PORT_FILE")
BASE="http://127.0.0.1:${PORT}"
CACHE="$WORKDIR/cache"

reset_counts() { : > "$COUNTS_FILE"; }
set_mode() { printf '%s\n' "$1" > "$MODE_FILE"; }

run_install() {
    DEST="$WORKDIR/dest"
    rm -rf "$DEST"
    mkdir -p "$DEST"
    env \
        GITHUB_RELEASE_BASE="$BASE" \
        ARMATURE_TOOL_CACHE="$CACHE" \
        ARMATURE_TOOL_FETCH_RETRIES="${ARMATURE_TOOL_FETCH_RETRIES:-5}" \
        ARMATURE_TOOL_FETCH_RETRY_MAX_TIME="${ARMATURE_TOOL_FETCH_RETRY_MAX_TIME:-30}" \
        "$SCRIPT" \
        --repo example/tool \
        --version v9.9.9 \
        --tarball 'tool-{ver}-{os}-{arch}.tar.gz' \
        --checksums 'tool-{ver}-checksums.txt' \
        --bin tool \
        --dest "$DEST"
}

echo "Test 1: matching checksum installs the binary..."
set_mode ok
reset_counts
rm -rf "$CACHE"
if run_install >/dev/null && [[ -x "$WORKDIR/dest/tool" ]]; then
    pass "installs on checksum match"
else
    fail "expected install on matching checksum"
fi

echo "Test 2: checksum mismatch fails..."
printf '%s  %s\n' "$(printf '0%.0s' {1..64})" "$TARBALL" \
    > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"
set_mode ok
reset_counts
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "checksum mismatch" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "mismatch fails the install"
else
    fail "expected checksum mismatch failure, status=$STATUS out=$OUT"
fi

echo "Test 3: missing checksums entry fails..."
printf '%s  other-file.tar.gz\n' "$HASH" \
    > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"
set_mode ok
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "no sha256 entry"; then
    pass "missing checksums entry fails"
else
    fail "expected missing-entry failure, status=$STATUS out=$OUT"
fi

echo "Test 4: one HTTP 504 then 200 still installs..."
write_good_sums
set_mode flaky
reset_counts
rm -rf "$CACHE"
export ARMATURE_TOOL_FETCH_RETRIES=2
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=20
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -eq 0 ]] && [[ -x "$WORKDIR/dest/tool" ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    pass "transient 504 then 200 installs the pinned binary"
else
    fail "expected install after transient 504, status=$STATUS out=$OUT"
fi
unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME

echo "Test 5: checksum mismatch still fails after retries exist..."
printf '%s  %s\n' "$(printf 'a%.0s' {1..64})" "$TARBALL" \
    > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"
set_mode ok
reset_counts
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "checksum mismatch" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "checksum mismatch fails closed"
else
    fail "expected hard failure on checksum mismatch, status=$STATUS out=$OUT"
fi

echo "Test 6: warm version+sha256 cache survives a later tarball 504..."
write_good_sums
set_mode ok
reset_counts
rm -rf "$CACHE"
if ! run_install >/dev/null; then
    fail "prime install failed"
else
    cached="$CACHE/example/tool/v9.9.9/$HASH/$TARBALL"
    if [[ ! -f "$cached" ]]; then
        fail "cache key repo/version/sha256/artifact missing at $cached"
    else
        set_mode tarball-504
        reset_counts
        export ARMATURE_TOOL_FETCH_RETRIES=1
        export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
        set +e
        OUT=$(run_install 2>&1)
        STATUS=$?
        set -e
        tarball_hits=$(rg -c "/$TARBALL$" "$COUNTS_FILE" || true)
        if [[ $STATUS -eq 0 ]] && [[ -x "$WORKDIR/dest/tool" ]] && [[ "${tarball_hits:-0}" -eq 0 ]]; then
            pass "cache hit verified and skipped the tarball download"
        else
            fail "expected cache hit without tarball GET, status=$STATUS hits=${tarball_hits:-0} out=$OUT"
        fi
        unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
    fi
fi

echo "Test 7: corrupted cache entry is not installed..."
write_good_sums
set_mode tarball-504
reset_counts
cached="$CACHE/example/tool/v9.9.9/$HASH/$TARBALL"
if [[ -f "$cached" ]]; then
    printf 'not-the-tarball\n' > "$cached"
    export ARMATURE_TOOL_FETCH_RETRIES=1
    export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
    set +e
    OUT=$(run_install 2>&1)
    STATUS=$?
    set -e
    unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
    if [[ $STATUS -ne 0 ]] && [[ ! -e "$WORKDIR/dest/tool" ]]; then
        pass "bad cache bytes are rejected when the tarball cannot be re-fetched"
    else
        fail "expected refusal of corrupted cache, status=$STATUS out=$OUT"
    fi
else
    fail "no cache file to corrupt"
fi

echo "Test 8: persistent HTTP 504 fails the fetch..."
write_good_sums
set_mode always-504
reset_counts
rm -rf "$CACHE"
export ARMATURE_TOOL_FETCH_RETRIES=1
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "download failed" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "exhausted retries fail closed"
else
    fail "expected hard failure after retries, status=$STATUS out=$OUT"
fi

echo "Test 9: checksums HTTP 504 uses the one verified cached artifact..."
write_good_sums
set_mode ok
reset_counts
rm -rf "$CACHE"
if ! run_install >/dev/null; then
    fail "prime install for checksums outage failed"
else
    set_mode always-504
    reset_counts
    export ARMATURE_TOOL_FETCH_RETRIES=1
    export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
    set +e
    OUT=$(run_install 2>&1)
    STATUS=$?
    set -e
    unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
    if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
        pass "single verified cache entry survives a checksums 504"
    else
        fail "expected cached artifact during checksums 504, status=$STATUS out=$OUT"
    fi
fi

echo "Test 10: two cached hashes and a checksums 504 fail closed..."
write_good_sums
set_mode ok
if [[ ! -f "$CACHE/example/tool/v9.9.9/$HASH/$TARBALL" ]]; then
    fail "expected primed cache before ambiguity check"
else
    python3 - "$CACHE/example/tool/v9.9.9" "$TARBALL" <<'PY'
import hashlib
import pathlib
import sys
root = pathlib.Path(sys.argv[1])
name = sys.argv[2]
data = b"second-artifact"
digest = hashlib.sha256(data).hexdigest()
dest = root / digest
dest.mkdir(parents=True)
(dest / name).write_bytes(data)
PY
    set_mode always-504
    export ARMATURE_TOOL_FETCH_RETRIES=1
    export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
    set +e
    OUT=$(run_install 2>&1)
    STATUS=$?
    set -e
    unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
    if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "no single verified" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
        pass "ambiguous cache is not installed"
    else
        fail "expected ambiguity failure, status=$STATUS out=$OUT"
    fi
fi

echo "Test 11: release-download 504 still installs via the pinned asset API..."
write_good_sums
set_mode release-504
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=1
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
release_hits=$(rg -c "/releases/download/" "$COUNTS_FILE" || true)
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]] && [[ "${release_hits:-0}" -eq 0 ]]; then
    pass "asset API installs the pinned binary without the download gateway"
else
    fail "expected asset API install, status=$STATUS release_hits=${release_hits:-0} out=$OUT"
fi

echo "Test 12: asset API bytes still fail closed on checksum mismatch..."
printf '%s  %s\n' "$(printf 'b%.0s' {1..64})" "$TARBALL" \
    > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"
set_mode release-504
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=1
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "checksum mismatch" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "asset API checksum mismatch fails closed"
else
    fail "expected checksum mismatch via asset API, status=$STATUS out=$OUT"
fi

echo "Test 13: one asset-API HTTP 403 then 200 still installs..."
write_good_sums
set_mode api-403-once
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=1
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=15
export ARMATURE_TOOL_FETCH_BACKOFF_SECS=0
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME ARMATURE_TOOL_FETCH_BACKOFF_SECS
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]] && echo "$OUT" | rg -q "HTTP 403"; then
    pass "transient asset API 403 is retried and checksum-verified"
else
    fail "expected install after asset API 403, status=$STATUS out=$OUT"
fi

kill "$SERVER_PID" 2>/dev/null || true

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All install-verified-github-release tests passed"
    exit 0
fi
echo "FAIL: $FAILURES install-verified-github-release test(s) failed"
exit 1
