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
tar -C "$WORKDIR/bin" -czf "$WORKDIR/www/example/tool/releases/download/v9.9.9/$TARBALL" tool
HASH=$(sha256sum "$WORKDIR/www/example/tool/releases/download/v9.9.9/$TARBALL" | awk '{print $1}')
printf '%s  %s\n' "$HASH" "$TARBALL" \
    > "$WORKDIR/www/example/tool/releases/download/v9.9.9/tool-9.9.9-checksums.txt"

PORT_FILE="$WORKDIR/port"
python3 - "$WORKDIR/www" "$PORT_FILE" <<'PY' &
import http.server
import os
import sys
from pathlib import Path

root = Path(sys.argv[1])
port_file = Path(sys.argv[2])
os.chdir(root)

class Handler(http.server.SimpleHTTPRequestHandler):
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

run_install() {
    DEST="$WORKDIR/dest"
    rm -rf "$DEST"
    mkdir -p "$DEST"
    GITHUB_RELEASE_BASE="$BASE" "$SCRIPT" \
        --repo example/tool \
        --version v9.9.9 \
        --tarball 'tool-{ver}-{os}-{arch}.tar.gz' \
        --checksums 'tool-{ver}-checksums.txt' \
        --bin tool \
        --dest "$DEST"
}

echo "Test 1: matching checksum installs the binary..."
if run_install >/dev/null && [[ -x "$WORKDIR/dest/tool" ]]; then
    pass "installs on checksum match"
else
    fail "expected install on matching checksum"
fi

echo "Test 2: checksum mismatch fails..."
printf '%s  %s\n' "$(printf '0%.0s' {1..64})" "$TARBALL" \
    > "$WORKDIR/www/example/tool/releases/download/v9.9.9/tool-9.9.9-checksums.txt"
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "checksum mismatch"; then
    pass "mismatch fails the install"
else
    fail "expected checksum mismatch failure, status=$STATUS out=$OUT"
fi

echo "Test 3: missing checksums entry fails..."
printf '%s  other-file.tar.gz\n' "$HASH" \
    > "$WORKDIR/www/example/tool/releases/download/v9.9.9/tool-9.9.9-checksums.txt"
set +e
OUT=$(run_install 2>&1)
STATUS=$?
set -e
if [[ $STATUS -ne 0 ]] && echo "$OUT" | rg -q "no sha256 entry"; then
    pass "missing checksums entry fails"
else
    fail "expected missing-entry failure, status=$STATUS out=$OUT"
fi

kill "$SERVER_PID" 2>/dev/null || true

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All install-verified-github-release tests passed"
    exit 0
fi
echo "FAIL: $FAILURES install-verified-github-release test(s) failed"
exit 1
