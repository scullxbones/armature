#!/usr/bin/env bash
# Prove scripts/install-windows-make.sh retries a transient fetch failure,
# falls through when one pinned source is down, and exits non-zero when
# every source is exhausted or a checksum/version check fails.
set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/install-windows-make.sh"
PIN="$REPO_ROOT/scripts/windows-make.sha256"

FAILURES=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; FAILURES=$((FAILURES + 1)); }

if [[ ! -x "$SCRIPT" ]]; then
    echo "FAIL: $SCRIPT is not executable"
    exit 1
fi
if [[ ! -f "$PIN" ]]; then
    echo "FAIL: missing $PIN"
    exit 1
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

BODY='#!/bin/sh
echo "GNU Make 4.4.1"
echo "Built for test"
'

write_exe() {
    local dest="$1" text="$2"
    printf '%s' "$text" > "$dest"
    chmod +x "$dest"
}

write_zip() {
    local dest="$1" inner="$2" exe="$3"
    python3 - "$dest" "$inner" "$exe" <<'PY'
import os, sys, zipfile
dest, inner, exe = sys.argv[1:]
info = zipfile.ZipInfo(inner)
info.create_system = 3
info.external_attr = (0o755 << 16) | 0x80000000
with zipfile.ZipFile(dest, "w") as zf:
    with open(exe, "rb") as fh:
        zf.writestr(info, fh.read())
PY
}

sha() { sha256sum "$1" | awk 'NR==1 { print $1 }'; }

write_exe "$WORKDIR/make.exe" "$BODY"
EXE_SHA=$(sha "$WORKDIR/make.exe")
write_zip "$WORKDIR/good-a.zip" "bin/make.exe" "$WORKDIR/make.exe"
write_zip "$WORKDIR/good-b.zip" "tools/install/bin/make.exe" "$WORKDIR/make.exe"
SHA_A=$(sha "$WORKDIR/good-a.zip")
SHA_B=$(sha "$WORKDIR/good-b.zip")

URL_A="https://downloads.example.test/make-4.4.1-without-guile-w32-bin.zip"
URL_B="https://community.example.test/api/v2/package/make/4.4.1"

write_pin() {
    local dest="$1"
    shift
    {
        echo "# exe-sha256: $EXE_SHA"
        echo "# version: 4.4.1"
        printf '%s\n' "$@"
    } > "$dest"
}

new_curl() {
    # $1 mode: fail-once | always-fail | bad-bytes | by-url
    local mode="$1"
    local curl_bin="$WORKDIR/curl-$mode"
    cat > "$curl_bin" <<EOF
#!/bin/bash
set -euo pipefail
dest=""
url=""
args=("\$@")
i=0
while [[ \$i -lt \${#args[@]} ]]; do
  a="\${args[\$i]}"
  case "\$a" in
    -o)
      i=\$((i + 1))
      dest="\${args[\$i]}"
      ;;
    -A|--connect-timeout|--max-time|--retry|--max-redirs|--proto-redir)
      i=\$((i + 1))
      ;;
    -*)
      ;;
    *)
      url="\$a"
      ;;
  esac
  i=\$((i + 1))
done
printf '%s\n' "\$url" >> "$WORKDIR/curl-log"
mode="$mode"
if [[ "\$mode" == "fail-once" ]]; then
  n=\$(wc -l < "$WORKDIR/curl-log")
  if [[ "\$n" -eq 1 ]]; then
    exit 22
  fi
fi
if [[ "\$mode" == "always-fail" ]]; then
  exit 22
fi
if [[ "\$mode" == "bad-bytes" ]]; then
  printf 'not-a-zip\n' > "\$dest"
  exit 0
fi
case "\$url" in
  "$URL_A") cp "$WORKDIR/good-a.zip" "\$dest" ;;
  "$URL_B") cp "$WORKDIR/good-b.zip" "\$dest" ;;
  *) exit 22 ;;
esac
EOF
    chmod +x "$curl_bin"
    printf '%s\n' "$curl_bin"
}

run_install() {
    local pin="$1" curl_bin="$2" attempts="${3:-4}"
    local root="$WORKDIR/gnumake-4.4.1" pathfile="$WORKDIR/github-path"
    rm -rf "$root"
    : > "$pathfile"
    : > "$WORKDIR/curl-log"
    set +e
    MAKE_PIN_FILE="$pin" \
    MAKE_CURL="$curl_bin" \
    MAKE_INSTALL_ROOT="$root" \
    MAKE_FETCH_ATTEMPTS="$attempts" \
    MAKE_FETCH_BACKOFF_SECS=0 \
    GITHUB_PATH="$pathfile" \
    bash "$SCRIPT" >"$WORKDIR/out" 2>"$WORKDIR/err"
    RUN_STATUS=$?
    set -e
    RUN_OUT=$(cat "$WORKDIR/out")
    RUN_ERR=$(cat "$WORKDIR/err")
    RUN_PATH=$(cat "$pathfile")
}

echo "Test 1: committed pin is GNU Make 4.4.1 from two https sources..."
if ! grep -q '^# exe-sha256: cc6dc291113dcbcc7735835acbfc23c52ed037e4e124ff2d7a0aeae6df563a9f$' "$PIN"; then
    fail "exe sha256 drifted from the ezwinports make.exe hash"
elif ! grep -q '^# version: 4.4.1$' "$PIN"; then
    fail "version pin is not 4.4.1"
elif ! grep -q '^fb66a02b530f7466f6222ce53c0b602c5288e601547a034e4156a512dd895ee7  https://downloads.sourceforge.net/project/ezwinports/make-4.4.1-without-guile-w32-bin.zip  bin/make.exe$' "$PIN"; then
    fail "sourceforge zip pin drifted"
elif ! grep -q '^8b2dec73ed4c84163c4a4f9bfe658553b1e3d1fdfa8930d2aa438e030ff21f9d  https://community.chocolatey.org/api/v2/package/make/4.4.1  tools/install/bin/make.exe$' "$PIN"; then
    fail "chocolatey nupkg pin drifted"
elif grep -q 'latest' "$PIN"; then
    fail "pin file contains latest"
fi
noncomment=$(grep -vE '^#|^$' "$PIN" || true)
if [[ "$(printf '%s\n' "$noncomment" | grep -c .)" -ne 2 ]]; then
    fail "expected exactly two archive sources"
elif printf '%s\n' "$noncomment" | grep -vq '  https://'; then
    fail "a source is not https"
else
    pass "pin file has two https archives and the make.exe sha256"
fi

echo "Test 2: a failed first attempt recovers..."
write_pin "$WORKDIR/pin-one" "$SHA_A  $URL_A  bin/make.exe"
CURL_BIN=$(new_curl fail-once)
run_install "$WORKDIR/pin-one" "$CURL_BIN" 4
calls=$(wc -l < "$WORKDIR/curl-log" | tr -d ' ')
if [[ "$RUN_STATUS" -ne 0 ]]; then
    fail "exit $RUN_STATUS; stderr: $RUN_ERR"
elif [[ "$calls" -ne 2 ]]; then
    fail "expected 2 fetches after one failure, got $calls"
elif ! printf '%s\n' "$RUN_OUT" | grep -q 'attempt 1/4'; then
    fail "missing attempt 1 log"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'retrying in 0s'; then
    fail "missing retry log"
elif ! printf '%s\n' "$RUN_OUT" | grep -qx 'GNU Make 4.4.1'; then
    fail "version output missing: $RUN_OUT"
elif ! printf '%s\n' "$RUN_PATH" | grep -q 'gnumake-4.4.1/bin'; then
    fail "GITHUB_PATH was not updated: $RUN_PATH"
else
    pass "first fetch failure retried and installed GNU Make 4.4.1"
fi

echo "Test 3: one dead source falls through to the other..."
write_pin "$WORKDIR/pin-two" \
    "$SHA_A  $URL_A  bin/make.exe" \
    "$SHA_B  $URL_B  tools/install/bin/make.exe"
cat > "$WORKDIR/curl-one-down" <<EOF
#!/bin/bash
set -euo pipefail
dest=""
url=""
args=("\$@")
i=0
while [[ \$i -lt \${#args[@]} ]]; do
  a="\${args[\$i]}"
  case "\$a" in
    -o) i=\$((i + 1)); dest="\${args[\$i]}" ;;
    -A|--connect-timeout|--max-time|--retry|--max-redirs|--proto-redir) i=\$((i + 1)) ;;
    -*) ;;
    *) url="\$a" ;;
  esac
  i=\$((i + 1))
done
printf '%s\n' "\$url" >> "$WORKDIR/curl-log"
if [[ "\$url" == "$URL_A" ]]; then
  exit 22
fi
cp "$WORKDIR/good-b.zip" "\$dest"
EOF
chmod +x "$WORKDIR/curl-one-down"
run_install "$WORKDIR/pin-two" "$WORKDIR/curl-one-down" 2
a_calls=$(grep -c -F "$URL_A" "$WORKDIR/curl-log" || true)
b_calls=$(grep -c -F "$URL_B" "$WORKDIR/curl-log" || true)
if [[ "$RUN_STATUS" -ne 0 ]]; then
    fail "exit $RUN_STATUS; stderr: $RUN_ERR"
elif [[ "$a_calls" -ne 2 || "$b_calls" -ne 1 ]]; then
    fail "expected source A x2 and B x1, got A=$a_calls B=$b_calls"
elif ! printf '%s\n' "$RUN_ERR" | grep -q "exhausted 2 attempts for $URL_A"; then
    fail "missing exhaustion log for the dead source"
elif ! printf '%s\n' "$RUN_OUT" | grep -qx 'GNU Make 4.4.1'; then
    fail "second source did not publish GNU Make 4.4.1"
else
    pass "a dead first source did not fail the install"
fi

echo "Test 4: exhausted retries exit non-zero and do not publish make..."
write_pin "$WORKDIR/pin-two" \
    "$SHA_A  $URL_A  bin/make.exe" \
    "$SHA_B  $URL_B  tools/install/bin/make.exe"
CURL_BIN=$(new_curl always-fail)
run_install "$WORKDIR/pin-two" "$CURL_BIN" 3
calls=$(wc -l < "$WORKDIR/curl-log" | tr -d ' ')
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "exhausted retries exited 0"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'FAIL: GNU Make 4.4.1 was not installed'; then
    fail "missing loud failure: $RUN_ERR"
elif ! printf '%s\n' "$RUN_ERR" | grep -q "exhausted 3 attempts for $URL_A"; then
    fail "missing per-source exhaustion: $RUN_ERR"
elif ! printf '%s\n' "$RUN_ERR" | grep -q "exhausted 3 attempts for $URL_B"; then
    fail "second source was not exhausted: $RUN_ERR"
elif [[ "$calls" -ne 6 ]]; then
    fail "expected 6 fetches, got $calls"
elif [[ -n "$RUN_PATH" ]]; then
    fail "GITHUB_PATH updated after failure: $RUN_PATH"
elif [[ -d "$WORKDIR/gnumake-4.4.1" ]]; then
    fail "install root left behind"
else
    pass "exhausted sources exited non-zero without publishing make"
fi

echo "Test 5: checksum mismatch is not retried..."
write_pin "$WORKDIR/pin-one" "$SHA_A  $URL_A  bin/make.exe"
CURL_BIN=$(new_curl bad-bytes)
run_install "$WORKDIR/pin-one" "$CURL_BIN" 4
calls=$(wc -l < "$WORKDIR/curl-log" | tr -d ' ')
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "bad checksum exited 0"
elif [[ "$calls" -ne 1 ]]; then
    fail "checksum mismatch was retried ($calls fetches)"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'checksum mismatch'; then
    fail "missing checksum mismatch: $RUN_ERR"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'not retrying this source'; then
    fail "mismatch should skip the source: $RUN_ERR"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'FAIL: GNU Make 4.4.1 was not installed'; then
    fail "mismatch did not fail the install: $RUN_ERR"
else
    pass "checksum mismatch failed closed without a retry"
fi

echo "Test 6: wrong make.exe bytes are rejected..."
write_exe "$WORKDIR/other.exe" '#!/bin/sh
echo "GNU Make 4.4.1"
'
write_zip "$WORKDIR/other.zip" "bin/make.exe" "$WORKDIR/other.exe"
SHA_OTHER=$(sha "$WORKDIR/other.zip")
write_pin "$WORKDIR/pin-other" "$SHA_OTHER  $URL_A  bin/make.exe"
cat > "$WORKDIR/curl-other" <<EOF
#!/bin/bash
set -euo pipefail
dest=""
url=""
args=("\$@")
i=0
while [[ \$i -lt \${#args[@]} ]]; do
  a="\${args[\$i]}"
  case "\$a" in
    -o) i=\$((i + 1)); dest="\${args[\$i]}" ;;
    -A|--connect-timeout|--max-time|--retry|--max-redirs|--proto-redir) i=\$((i + 1)) ;;
    -*) ;;
    *) url="\$a" ;;
  esac
  i=\$((i + 1))
done
printf '%s\n' "\$url" >> "$WORKDIR/curl-log"
cp "$WORKDIR/other.zip" "\$dest"
EOF
chmod +x "$WORKDIR/curl-other"
run_install "$WORKDIR/pin-other" "$WORKDIR/curl-other" 4
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "wrong make.exe exited 0"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'make.exe checksum mismatch'; then
    fail "missing exe checksum error: $RUN_ERR"
elif [[ -n "$RUN_PATH" ]]; then
    fail "GITHUB_PATH updated for the wrong binary"
else
    pass "make.exe hash is checked before PATH"
fi

echo "Test 7: a verified binary that is not GNU Make 4.4.1 is not published..."
write_exe "$WORKDIR/wrong-ver.exe" '#!/bin/sh
echo "GNU Make 9.9.9"
'
WRONG_SHA=$(sha "$WORKDIR/wrong-ver.exe")
write_zip "$WORKDIR/wrong-ver.zip" "bin/make.exe" "$WORKDIR/wrong-ver.exe"
SHA_WRONG=$(sha "$WORKDIR/wrong-ver.zip")
{
    echo "# exe-sha256: $WRONG_SHA"
    echo "# version: 4.4.1"
    echo "$SHA_WRONG  $URL_A  bin/make.exe"
} > "$WORKDIR/pin-wrong-ver"
cat > "$WORKDIR/curl-wrong-ver" <<EOF
#!/bin/bash
set -euo pipefail
dest=""
args=("\$@")
i=0
while [[ \$i -lt \${#args[@]} ]]; do
  a="\${args[\$i]}"
  case "\$a" in
    -o) i=\$((i + 1)); dest="\${args[\$i]}" ;;
    -A|--connect-timeout|--max-time|--retry|--max-redirs|--proto-redir) i=\$((i + 1)) ;;
    -*) ;;
  esac
  i=\$((i + 1))
done
echo hit >> "$WORKDIR/curl-log"
cp "$WORKDIR/wrong-ver.zip" "\$dest"
EOF
chmod +x "$WORKDIR/curl-wrong-ver"
run_install "$WORKDIR/pin-wrong-ver" "$WORKDIR/curl-wrong-ver" 4
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "wrong version exited 0"
elif ! printf '%s\n' "$RUN_ERR" | grep -q "reported 'GNU Make 9.9.9'"; then
    fail "missing version failure: $RUN_ERR"
elif [[ -n "$RUN_PATH" ]]; then
    fail "GITHUB_PATH updated for the wrong version"
elif [[ -e "$WORKDIR/gnumake-4.4.1/bin/make.exe" ]]; then
    fail "wrong make.exe was left installed"
else
    pass "version mismatch removed the binary and exited non-zero"
fi

echo "Test 8: non-https sources are refused before any fetch..."
write_pin "$WORKDIR/pin-http" "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  http://downloads.example.test/make.zip  bin/make.exe"
# exe sha in write_pin is the good exe; archive hash is unused because URL is refused.
CURL_BIN=$(new_curl always-fail)
run_install "$WORKDIR/pin-http" "$CURL_BIN" 2
calls=$(wc -l < "$WORKDIR/curl-log" | tr -d ' ')
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "http source exited 0"
elif [[ "$calls" -ne 0 ]]; then
    fail "http source was fetched"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'refusing non-https'; then
    fail "missing https refusal: $RUN_ERR"
else
    pass "non-https source refused without a fetch"
fi

echo "Test 9: a non-positive attempt budget fails closed..."
write_pin "$WORKDIR/pin-one" "$SHA_A  $URL_A  bin/make.exe"
CURL_BIN=$(new_curl fail-once)
run_install "$WORKDIR/pin-one" "$CURL_BIN" 0
if [[ "$RUN_STATUS" -eq 0 ]]; then
    fail "zero attempts exited 0"
elif ! printf '%s\n' "$RUN_ERR" | grep -q 'MAKE_FETCH_ATTEMPTS must be between 1 and 8'; then
    fail "missing attempt-budget error: $RUN_ERR"
else
    pass "attempt budget is bounded"
fi

if [[ "$FAILURES" -ne 0 ]]; then
    echo "FAIL: $FAILURES test(s)"
    exit 1
fi
echo "OK: install-windows-make"
