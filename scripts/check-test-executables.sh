#!/usr/bin/env bash
# Fail when a _test.go writes an executable outside testexec.Write (golang/go#22315).

set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-test-executables" >&2
    exit 1
fi

mapfile -t test_files < <(rg --files --glob '*_test.go' --glob '!internal/testexec/**' . | LC_ALL=C sort)
if [[ ${#test_files[@]} -eq 0 ]]; then
    echo "OK: no _test.go writes an executable outside testexec.Write"
    exit 0
fi

hits=$(awk -f /dev/stdin "${test_files[@]}" <<'AWK'
function skip_space() {
    while (i <= n) {
        c = substr(src, i, 1)
        if (c == " " || c == "\t" || c == "\r" || c == "\n") { i++; continue }
        if (c == "/" && substr(src, i, 2) == "//") {
            i += 2
            while (i <= n && substr(src, i, 1) != "\n") i++
            continue
        }
        if (c == "/" && substr(src, i, 2) == "/*") {
            i += 2
            while (i <= n && substr(src, i, 2) != "*/") i++
            i += 2
            continue
        }
        break
    }
}

function parse_string(   q, esc, ch) {
    q = substr(src, i, 1)
    i++
    if (q == "`") {
        while (i <= n && substr(src, i, 1) != "`") i++
        if (i <= n) i++
        return
    }
    esc = 0
    while (i <= n) {
        ch = substr(src, i, 1)
        i++
        if (esc) { esc = 0; continue }
        if (ch == "\\") { esc = 1; continue }
        if (ch == q) return
    }
}

function capture_call(   depth, start, ch) {
    skip_space()
    if (substr(src, i, 1) != "(") return ""
    depth = 0
    start = i
    while (i <= n) {
        ch = substr(src, i, 1)
        if (ch == "\"" || ch == "'" || ch == "`") { parse_string(); continue }
        if (ch == "/" && substr(src, i, 2) == "//") {
            while (i <= n && substr(src, i, 1) != "\n") i++
            continue
        }
        if (ch == "/" && substr(src, i, 2) == "/*") {
            i += 2
            while (i <= n && substr(src, i, 2) != "*/") i++
            i += 2
            continue
        }
        if (ch == "(") depth++
        else if (ch == ")") {
            depth--
            i++
            if (depth == 0) return substr(src, start, i - start)
            continue
        }
        i++
    }
    return ""
}

function split_args(call, args,    inner, d, j, ch, astart, narg, esc, in_str) {
    inner = call
    sub(/^\(/, "", inner)
    sub(/\)$/, "", inner)
    narg = 0
    d = 0
    astart = 1
    esc = 0
    in_str = ""
    for (j = 1; j <= length(inner); j++) {
        ch = substr(inner, j, 1)
        if (in_str != "") {
            if (esc) { esc = 0; continue }
            if (in_str != "`" && ch == "\\") { esc = 1; continue }
            if (ch == in_str) in_str = ""
            continue
        }
        if (ch == "\"" || ch == "'" || ch == "`") { in_str = ch; continue }
        if (ch == "(" || ch == "[" || ch == "{") { d++; continue }
        if (ch == ")" || ch == "]" || ch == "}") { d--; continue }
        if (ch == "," && d == 0) {
            narg++
            args[narg] = trim(substr(inner, astart, j - astart))
            astart = j + 1
        }
    }
    narg++
    args[narg] = trim(substr(inner, astart))
    return narg
}

function trim(s) {
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
    gsub(/[[:space:]]+/, " ", s)
    return s
}

function oct_to_dec(s,    k, v) {
    v = 0
    for (k = 1; k <= length(s); k++) v = v * 8 + substr(s, k, 1)
    return v
}

function octal_has_exec(tok,    raw, n) {
    raw = tok
    if (raw ~ /^0[oO][0-7]+$/) sub(/^0[oO]/, "", raw)
    else if (raw ~ /^0[0-7]+$/) sub(/^0/, "", raw)
    else return 0
    if (raw == "") raw = "0"
    n = oct_to_dec(raw)
    return (int(n / 64) % 2 == 1) || (int(n / 8) % 2 == 1) || (n % 2 == 1)
}

function mode_has_exec(expr,    t, rest) {
    if (expr ~ /ModePerm/) return 1
    rest = expr
    while (match(rest, /0[oO][0-7]+|0[0-7]+/)) {
        t = substr(rest, RSTART, RLENGTH)
        if (octal_has_exec(t)) return 1
        rest = substr(rest, RSTART + RLENGTH)
    }
    return 0
}

function line_at(pos,    j, line) {
    line = 1
    for (j = 1; j < pos && j <= n; j++) if (substr(src, j, 1) == "\n") line++
    return line
}

function record(kind, pos, detail) {
    nhits++
    hits[nhits] = file ":" line_at(pos) ": " kind " " detail
}

function path_was_written(path,    w) {
    for (w = 1; w <= nwrite; w++) if (writes[w] == path) return 1
    return 0
}

function walk(pass,    pos, call, nargs, args) {
    i = 1
    while (i <= n) {
        if (substr(src, i, 1) == "\"" || substr(src, i, 1) == "'" || substr(src, i, 1) == "`") { parse_string(); continue }
        if (substr(src, i, 2) == "//") {
            while (i <= n && substr(src, i, 1) != "\n") i++
            continue
        }
        if (substr(src, i, 2) == "/*") {
            i += 2
            while (i <= n && substr(src, i, 2) != "*/") i++
            i += 2
            continue
        }
        if (substr(src, i, 13) == "os.WriteFile(") {
            pos = i
            i += 12
            call = capture_call()
            if (call == "") continue
            nargs = split_args(call, args)
            if (pass == 1 && nargs >= 1) writes[++nwrite] = args[1]
            if (pass == 2 && nargs >= 3 && mode_has_exec(args[3])) record("os.WriteFile", pos, args[3])
            continue
        }
        if (substr(src, i, 12) == "os.OpenFile(") {
            pos = i
            i += 11
            call = capture_call()
            if (call == "") continue
            nargs = split_args(call, args)
            if (pass == 1 && nargs >= 1) writes[++nwrite] = args[1]
            if (pass == 2 && nargs >= 3 && mode_has_exec(args[3])) record("os.OpenFile", pos, args[3])
            continue
        }
        if (substr(src, i, 9) == "os.Chmod(") {
            pos = i
            i += 8
            call = capture_call()
            if (call == "") continue
            if (pass != 2) continue
            nargs = split_args(call, args)
            if (nargs >= 2 && mode_has_exec(args[2]) && path_was_written(args[1])) record("os.Chmod", pos, args[2])
            continue
        }
        if (substr(src, i, 6) == ".Chmod(") {
            pos = i
            i += 5
            call = capture_call()
            if (call == "") continue
            if (pass != 2) continue
            nargs = split_args(call, args)
            if (nargs >= 1 && mode_has_exec(args[1]) && nwrite > 0) record("Chmod", pos, args[1])
            continue
        }
        i++
    }
}

function scan() {
    n = length(src)
    nwrite = 0
    delete writes
    walk(1)
    walk(2)
}

BEGIN { nhits = 0 }

FNR == 1 {
    if (NR != 1) scan()
    file = FILENAME
    src = ""
}

{ src = src $0 "\n" }

END {
    if (src != "") scan()
    for (h = 1; h <= nhits; h++) print hits[h]
}
AWK
)

if [[ -n "$hits" ]]; then
    echo "FAIL: _test.go writes an executable; use testexec.Write so tests never create an executable inode after init (golang/go#22315):" >&2
    echo "$hits" >&2
    exit 1
fi

echo "OK: no _test.go writes an executable outside testexec.Write"
