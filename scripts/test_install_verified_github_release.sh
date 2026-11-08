#!/usr/bin/env bash
# The installer trusts only ARMATURE_TOOL_PIN_FILE. These tests use a local
# stub: transient 504, a forged self-hashed cache, an https redirect with
# auth stripped, and a truncated 200 that must not win over a good fallback.
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
# A downloaded checksum file that lies. The installer must not consult it.
printf '%s  %s\n' "$(printf '0%.0s' {1..64})" "$TARBALL" > "$WORKDIR/www/$REL/tool-9.9.9-checksums.txt"

PIN_FILE="$WORKDIR/pins.txt"
write_pin() {
    printf '%s  %s  %s  %s\n' "$1" "$TARBALL" "example/tool" "v9.9.9" > "$PIN_FILE"
}
write_pin "$HASH"

MODE_FILE="$WORKDIR/mode"
AUTH_FILE="$WORKDIR/auth"
PORT_FILE="$WORKDIR/port"
TLS_PORT_FILE="$WORKDIR/tls-port"
printf 'ok\n' > "$MODE_FILE"
: > "$AUTH_FILE"

openssl req -x509 -newkey rsa:2048 -keyout "$WORKDIR/key.pem" -out "$WORKDIR/cert.pem" \
    -days 1 -nodes -subj /CN=localhost \
    -addext subjectAltName=DNS:localhost,IP:127.0.0.1 >/dev/null 2>&1

python3 - "$WORKDIR" <<'PY' &
import http.server, json, os, ssl, sys
from pathlib import Path

root = Path(sys.argv[1])
mode_file = root / "mode"
auth_file = root / "auth"
port_file = root / "port"
tls_port_file = root / "tls-port"
www = root / "www"
tarball_rel = "example/tool/releases/download/v9.9.9/tool-9.9.9-linux-amd64.tar.gz"

def note(handler):
    auth = handler.headers.get("Authorization")
    with auth_file.open("a", encoding="utf-8") as fh:
        fh.write(f"{handler.path} host={handler.headers.get('Host')} auth={auth}\n")

class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

    def send_bytes(self, status, body, content_type="application/octet-stream", extra=None):
        data = body if isinstance(body, bytes) else body.encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        if extra:
            for k, v in extra:
                self.send_header(k, v)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if data:
            self.wfile.write(data)

    def do_GET(self):
        note(self)
        mode = mode_file.read_text(encoding="utf-8").strip()
        path = self.path.split("?", 1)[0]
        tls_port = tls_port_file.read_text(encoding="utf-8").strip() if tls_port_file.exists() else "0"
        if path == "/artifact" or path.startswith("/edge/"):
            self.send_bytes(200, (www / tarball_rel).read_bytes())
            return
        if mode == "custom-redir":
            if path.startswith("/api/"):
                self.send_bytes(500, "api down", "text/plain")
                return
            if path.endswith(tarball_rel):
                loc = (root / "location").read_text(encoding="utf-8").strip()
                self.send_bytes(302, "", "text/plain", [("Location", loc)])
                return
        if path.startswith("/api/") and mode in ("redirect", "http-redirect"):
            self.send_bytes(500, "api down", "text/plain")
            return
        if mode == "api-flaky" and path.startswith("/api/repos/"):
            counts = root / "counts"
            prior = 0
            if counts.exists():
                prior = sum(1 for line in counts.read_text(encoding="utf-8").splitlines() if line == path)
            with counts.open("a", encoding="utf-8") as fh:
                fh.write(path + "\n")
            if prior == 0:
                self.send_bytes(504, "gateway timeout", "text/plain")
                return
            host = self.headers.get("Host")
            body = json.dumps({"assets": [{
                "name": "tool-9.9.9-linux-amd64.tar.gz",
                "url": f"http://{host}/api/asset/tarball",
            }]})
            self.send_bytes(200, body, "application/json")
            return
        if mode == "api-flaky" and path == "/api/asset/tarball":
            self.send_bytes(200, (www / tarball_rel).read_bytes())
            return
        if mode == "api-flaky" and path.endswith(tarball_rel):
            self.send_bytes(504, "gateway timeout", "text/plain")
            return
        if path == "/api/repos/example/tool/releases/tags/v9.9.9" and mode == "partial":
            host = self.headers.get("Host")
            body = json.dumps({"assets": [{
                "name": "tool-9.9.9-linux-amd64.tar.gz",
                "url": f"http://{host}/api/asset/tarball",
            }]})
            self.send_bytes(200, body, "application/json")
            return
        if path == "/api/asset/tarball" and mode == "partial":
            self.send_response(200)
            self.send_header("Content-Length", "500000")
            self.end_headers()
            self.wfile.write(b"short")
            self.close_connection = True
            return
        if path.endswith(tarball_rel) and mode == "redirect":
            self.send_bytes(302, "", "text/plain", [("Location", f"https://localhost:{tls_port}/artifact")])
            return
        if path.endswith(tarball_rel) and mode == "http-redirect":
            host = self.headers.get("Host")
            self.send_bytes(302, "", "text/plain", [("Location", f"http://{host}/files/{tarball_rel}")])
            return
        if path.startswith("/files/") and mode == "http-redirect":
            rel = path[len("/files/"):]
            self.send_bytes(200, (www / rel).read_bytes())
            return
        # Ordinary file serving, with optional one-shot 504s.
        target = www / path.lstrip("/")
        counts = root / "counts"
        prior = 0
        if counts.exists():
            prior = sum(1 for line in counts.read_text(encoding="utf-8").splitlines() if line == path)
        with counts.open("a", encoding="utf-8") as fh:
            fh.write(path + "\n")
        n = prior + 1
        if mode == "always-504" or (mode == "flaky" and n == 1):
            self.send_bytes(504, "gateway timeout", "text/plain")
            return
        if not target.is_file():
            self.send_bytes(404, "missing", "text/plain")
            return
        self.send_bytes(200, target.read_bytes())

def serve(httpd, port_path):
    port_path.write_text(str(httpd.server_address[1]), encoding="utf-8")
    httpd.serve_forever()

plain = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(root / "cert.pem", root / "key.pem")
tls = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
tls.socket = ctx.wrap_socket(tls.socket, server_side=True)
import threading
threading.Thread(target=serve, args=(plain, port_file), daemon=True).start()
threading.Thread(target=serve, args=(tls, tls_port_file), daemon=True).start()
threading.Event().wait()
PY
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true; rm -rf "$WORKDIR"' EXIT

for _ in $(seq 1 50); do
    if [[ -s "$PORT_FILE" && -s "$TLS_PORT_FILE" ]]; then
        break
    fi
    sleep 0.05
done
PORT=$(cat "$PORT_FILE")
BASE="http://127.0.0.1:${PORT}"
CACHE="$WORKDIR/cache"

set_mode() { printf '%s\n' "$1" > "$MODE_FILE"; }
reset_counts() { : > "$WORKDIR/counts"; : > "$AUTH_FILE"; }

run_install() {
    local dest="$WORKDIR/dest"
    rm -rf "$dest"
    mkdir -p "$dest"
    env \
        GITHUB_RELEASE_BASE="$BASE" \
        ARMATURE_TOOL_CACHE="$CACHE" \
        ARMATURE_TOOL_PIN_FILE="$PIN_FILE" \
        ARMATURE_TOOL_FETCH_RETRIES="${ARMATURE_TOOL_FETCH_RETRIES:-1}" \
        ARMATURE_TOOL_FETCH_RETRY_MAX_TIME="${ARMATURE_TOOL_FETCH_RETRY_MAX_TIME:-20}" \
        ARMATURE_TOOL_FETCH_BACKOFF_SECS="${ARMATURE_TOOL_FETCH_BACKOFF_SECS:-0}" \
        "$SCRIPT" \
        --repo example/tool \
        --version v9.9.9 \
        --tarball 'tool-{ver}-{os}-{arch}.tar.gz' \
        --bin tool \
        --dest "$dest"
}

echo "Test 1: pinned sha256 installs; a lying checksum file is not fetched..."
set_mode ok
reset_counts
rm -rf "$CACHE"
if run_install >/dev/null && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    if rg -q 'checksums.txt' "$WORKDIR/counts"; then
        fail "installer fetched a checksum file"
    else
        pass "install trusts the pin, not a checksum file"
    fi
else
    fail "expected install from the pinned digest"
fi

echo "Test 2: bytes that do not match the pin fail closed..."
write_pin "$(printf 'a%.0s' {1..64})"
set_mode ok
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
write_pin "$HASH"
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "checksum mismatch" && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "checksum mismatch fails the install"
else
    fail "expected checksum mismatch, status=$STATUS out=$OUT"
fi

echo "Test 3: missing pin fails..."
printf '%s  other.tar.gz  example/tool  v9.9.9\n' "$HASH" > "$PIN_FILE"
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
write_pin "$HASH"
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "no pinned sha256"; then
    pass "missing pin fails"
else
    fail "expected missing pin failure, status=$STATUS out=$OUT"
fi

echo "Test 4: one HTTP 504 then 200 still installs..."
set_mode flaky
reset_counts
rm -rf "$CACHE"
export ARMATURE_TOOL_FETCH_RETRIES=2
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    pass "transient 504 then 200 installs the pinned binary"
else
    fail "expected install after transient 504, status=$STATUS out=$OUT"
fi

echo "Test 5: persistent HTTP 504 fails..."
set_mode always-504
rm -rf "$CACHE"
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "exhausted fetch fails closed"
else
    fail "expected failure on persistent 504, status=$STATUS out=$OUT"
fi

echo "Test 6: cache entry that matches the pin survives a 504..."
set_mode ok
rm -rf "$CACHE"
run_install >/dev/null
set_mode always-504
export ARMATURE_TOOL_FETCH_RETRIES=0
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    pass "pinned cache hit is installed"
else
    fail "expected pinned cache hit, status=$STATUS out=$OUT"
fi

echo "Test 7: bytes at the pin path that do not match are not installed..."
printf 'not-the-tarball\n' > "$CACHE/example/tool/v9.9.9/$HASH/$TARBALL"
set_mode always-504
export ARMATURE_TOOL_FETCH_RETRIES=0
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -ne 0 ]] && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "corrupt pin-path cache is rejected"
else
    fail "expected rejection of corrupt cache, status=$STATUS out=$OUT"
fi

echo "Test 8: a lone self-hashed forged cache entry is rejected..."
rm -rf "$CACHE"
forged="$WORKDIR/forged"
printf 'forged-tarball\n' > "$forged"
forged_hash=$(sha256sum "$forged" | awk '{print $1}')
mkdir -p "$CACHE/example/tool/v9.9.9/$forged_hash"
cp "$forged" "$CACHE/example/tool/v9.9.9/$forged_hash/$TARBALL"
set_mode always-504
export ARMATURE_TOOL_FETCH_RETRIES=0
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -ne 0 ]] && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "forged self-hashed cache is rejected"
else
    fail "expected forged cache rejection, status=$STATUS out=$OUT"
fi

echo "Test 9: https redirect installs, and a job token is never sent..."
set_mode redirect
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_CACERT="$WORKDIR/cert.pem"
export ARMATURE_TOOL_FETCH_RETRIES=0
export GITHUB_TOKEN=pin-test-token
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_CACERT ARMATURE_TOOL_FETCH_RETRIES GITHUB_TOKEN
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]] \
    && ! rg -q 'pin-test-token' "$AUTH_FILE"; then
    pass "https redirect installed the pin without sending a token"
else
    fail "expected token-free https redirect, status=$STATUS auth=$(cat "$AUTH_FILE") out=$OUT"
fi

echo "Test 10: http redirect is refused even when the target matches the pin..."
set_mode http-redirect
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=0
export GITHUB_TOKEN=pin-test-token
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES GITHUB_TOKEN
if [[ $STATUS -ne 0 ]] && ! rg -q '/files/' "$AUTH_FILE" && ! rg -q 'pin-test-token' "$AUTH_FILE" \
    && [[ ! -e "$WORKDIR/dest/tool" ]]; then
    pass "http redirect is refused"
else
    fail "expected http redirect refusal, status=$STATUS auth=$(cat "$AUTH_FILE") out=$OUT"
fi

echo "Test 10b: fragment, userinfo, and @-authority redirects never carry the token..."
TLS_PORT=$(cat "$TLS_PORT_FILE")
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_CACERT="$WORKDIR/cert.pem"
export ARMATURE_TOOL_FETCH_RETRIES=0
export GITHUB_TOKEN=pin-test-token
token_leak=0
for loc in \
    "https://localhost:${TLS_PORT}/edge/fragment#@api.github.com/releases" \
    "https://user:pass@localhost:${TLS_PORT}/edge/userinfo" \
    "https://api.github.com@localhost:${TLS_PORT}/edge/at-sign"
do
    printf '%s\n' "$loc" > "$WORKDIR/location"
    set_mode custom-redir
    reset_counts
    rm -rf "$CACHE" "$WORKDIR/dest"
    set +e
    OUT=$(run_install 2>&1)
    STATUS=$?
    set -e
    if [[ $STATUS -ne 0 ]] || [[ "$( "$WORKDIR/dest/tool" 2>/dev/null || true )" != "fake-tool" ]] \
        || rg -q 'pin-test-token' "$AUTH_FILE"; then
        token_leak=1
        fail "token reached a non-GitHub-looking redirect ($loc) status=$STATUS auth=$(cat "$AUTH_FILE") out=$OUT"
    fi
done
unset ARMATURE_GITHUB_API ARMATURE_TOOL_CACERT ARMATURE_TOOL_FETCH_RETRIES GITHUB_TOKEN
if [[ "$token_leak" -eq 0 ]]; then
    pass "fragment, userinfo, and @-authority redirects did not receive the token"
fi

echo "Test 11: truncated 200 falls through to the release URL..."
set_mode partial
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=0
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    pass "truncated transfer fell through to a verified body"
else
    fail "expected fallback after truncated 200, status=$STATUS out=$OUT"
fi

echo "Test 12: one asset-API 504 is retried when the release URL stays down..."
set_mode api-flaky
reset_counts
rm -rf "$CACHE"
export ARMATURE_GITHUB_API="${BASE}/api"
export ARMATURE_TOOL_FETCH_RETRIES=2
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
unset ARMATURE_GITHUB_API ARMATURE_TOOL_FETCH_RETRIES
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]] && echo "$OUT" | rg -q "asset API"; then
    pass "asset API 504 was retried and matched the pin"
else
    fail "expected asset API retry, status=$STATUS out=$OUT"
fi

echo "Test 13: a symlink at the old predictable cache temp path is not followed..."
set_mode ok
reset_counts
rm -rf "$CACHE"
sentinel="$WORKDIR/sentinel"
printf 'sentinel-contents\n' > "$sentinel"
pin_dir="$CACHE/example/tool/v9.9.9/$HASH"
mkdir -p "$pin_dir"
ln -s "$sentinel" "$pin_dir/$TARBALL.tmp"
if run_install >/dev/null && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]] \
    && [[ "$(cat "$sentinel")" == "sentinel-contents" ]]; then
    pass "predictable cache temp symlink was not overwritten"
else
    fail "cache publish followed a temp symlink (sentinel=$(cat "$sentinel" 2>/dev/null))"
fi

echo "Test 14: two installers can publish the same cache entry..."
set_mode ok
rm -rf "$CACHE" "$WORKDIR/dest-a" "$WORKDIR/dest-b"
mkdir -p "$WORKDIR/dest-a" "$WORKDIR/dest-b"
env \
    GITHUB_RELEASE_BASE="$BASE" \
    ARMATURE_TOOL_CACHE="$CACHE" \
    ARMATURE_TOOL_PIN_FILE="$PIN_FILE" \
    ARMATURE_TOOL_FETCH_RETRIES=0 \
    ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=20 \
    ARMATURE_TOOL_FETCH_BACKOFF_SECS=0 \
    "$SCRIPT" --repo example/tool --version v9.9.9 \
    --tarball 'tool-{ver}-{os}-{arch}.tar.gz' --bin tool --dest "$WORKDIR/dest-a" \
    >"$WORKDIR/conc-a.out" 2>&1 &
pid_a=$!
env \
    GITHUB_RELEASE_BASE="$BASE" \
    ARMATURE_TOOL_CACHE="$CACHE" \
    ARMATURE_TOOL_PIN_FILE="$PIN_FILE" \
    ARMATURE_TOOL_FETCH_RETRIES=0 \
    ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=20 \
    ARMATURE_TOOL_FETCH_BACKOFF_SECS=0 \
    "$SCRIPT" --repo example/tool --version v9.9.9 \
    --tarball 'tool-{ver}-{os}-{arch}.tar.gz' --bin tool --dest "$WORKDIR/dest-b" \
    >"$WORKDIR/conc-b.out" 2>&1 &
pid_b=$!
set +e
wait "$pid_a"; st_a=$?
wait "$pid_b"; st_b=$?
set -e
cached="$CACHE/example/tool/v9.9.9/$HASH/$TARBALL"
if [[ $st_a -eq 0 && $st_b -eq 0 ]] \
    && [[ "$( "$WORKDIR/dest-a/tool" )" == "fake-tool" ]] \
    && [[ "$( "$WORKDIR/dest-b/tool" )" == "fake-tool" ]] \
    && [[ -f "$cached" ]] && [[ "$(sha256sum "$cached" | awk '{print $1}')" == "$HASH" ]]; then
    pass "concurrent installs both succeeded and the cache matches the pin"
else
    fail "concurrent cache publish failed a=$st_a b=$st_b"
fi

echo "Test 15: an unwritable cache does not fail the install..."
set_mode ok
rm -rf "$CACHE"
mkdir -p "$CACHE"
chmod a-w "$CACHE"
if [[ -w "$CACHE" ]]; then
    chmod u+w "$CACHE" 2>/dev/null || true
    rm -rf "$CACHE"
    printf 'not-a-directory\n' > "$CACHE"
fi
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
chmod u+w "$CACHE" 2>/dev/null || true
if [[ $STATUS -eq 0 ]] && [[ "$( "$WORKDIR/dest/tool" )" == "fake-tool" ]]; then
    pass "unwritable cache is ignored"
else
    fail "unwritable cache failed the install, status=$STATUS out=$OUT"
fi

echo "Test 16: backoff is clamped to the remaining budget..."
set_mode always-504
rm -rf "$CACHE"
export ARMATURE_TOOL_FETCH_RETRIES=1
export ARMATURE_TOOL_FETCH_RETRY_MAX_TIME=1
export ARMATURE_TOOL_FETCH_BACKOFF_SECS=30
start=$(date +%s)
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
end=$(date +%s)
unset ARMATURE_TOOL_FETCH_RETRIES ARMATURE_TOOL_FETCH_RETRY_MAX_TIME ARMATURE_TOOL_FETCH_BACKOFF_SECS
elapsed=$((end - start))
if [[ $STATUS -ne 0 && "$elapsed" -lt 8 && ! -e "$WORKDIR/dest/tool" ]]; then
    pass "30s backoff was clamped inside a 1s budget (${elapsed}s)"
else
    fail "backoff exceeded the budget, elapsed=${elapsed}s status=$STATUS out=$OUT"
fi

kill "$SERVER_PID" 2>/dev/null || true

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All install-verified-github-release tests passed"
    exit 0
fi
echo "FAIL: $FAILURES install-verified-github-release test(s) failed"
exit 1
