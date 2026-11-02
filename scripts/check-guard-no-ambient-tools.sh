#!/usr/bin/env bash
# Static check: production guard scripts must not bare-call external tools.
# Every shell-out goes through guard_resolve_tool → "$TOOL" absolute path.
#
# CLASS: real command-position analysis (not a closed tool-name list).
# Transparent prefixes: if/!/while/until/then/else/elif/do/env/time/exec/command
# and VAR=value assignments. Ban every non-builtin external not via resolver.
# Does not truncate at `#` inside `${#…}` or quotes.
#
# Auto-discovers guard-related scripts (resolver users + resolver + run-selftest).
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

python3 - "$ROOT" <<'PY'
import pathlib, re, sys

ROOT = pathlib.Path(".")

BUILTINS = {
    "[", "[[", "]", "]]", ":", ".", "alias", "bg", "bind", "break", "builtin",
    "caller", "cd", "command", "compgen", "complete", "compopt", "continue",
    "declare", "dirs", "disown", "echo", "enable", "eval", "exec", "exit",
    "export", "false", "fc", "fg", "getopts", "hash", "help", "history",
    "jobs", "kill", "let", "local", "logout", "mapfile", "popd", "printf",
    "pushd", "pwd", "read", "readarray", "readonly", "return", "set", "shift",
    "shopt", "source", "suspend", "test", "times", "trap", "true", "type",
    "typeset", "ulimit", "umask", "unalias", "unset", "wait",
    "if", "then", "else", "elif", "fi", "case", "esac", "for", "select",
    "while", "until", "do", "done", "in", "function", "time", "coproc",
    "!", "{", "}",
}
TRANSPARENT = {
    "if", "!", "while", "until", "then", "else", "elif", "do",
    "env", "time", "exec", "command",
}
IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_-]*$")
FUNC_DEF_RE = re.compile(
    r"(?:^|\n)\s*(?:function\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)\s*\{"
)
ASSIGN_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=(.*)$")
PINNED_VAR_RE = re.compile(r'^"\$[A-Z][A-Z0-9_]*"$|^\$[A-Z][A-Z0-9_]*$')
ABS_PIN_RE = re.compile(r"^/(?:usr/)?bin/[A-Za-z0-9._+-]+$")


SOURCE_RESOLVER_RE = re.compile(
    r"""\.\s+["']\$\{?[A-Za-z_][A-Za-z0-9_]*\}?/guard-resolve-tool\.sh["']"""
)


def discover(root: pathlib.Path):
    found = []
    scripts = root / "scripts"
    if not scripts.is_dir():
        return found
    for p in sorted(scripts.rglob("*")):
        if not p.is_file():
            continue
        rel = str(p.relative_to(root)).replace("\\", "/")
        if p.name == "check-guard-no-ambient-tools.sh":
            continue
        # Tests mention resolver in strings — not production guards.
        if "/test_" in rel or rel.startswith("scripts/test_"):
            continue
        if rel.startswith("scripts/testdata/") and not rel.endswith(
            "porcelain-bypass/run-selftest.sh"
        ):
            continue
        try:
            text = p.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        if p.name == "guard-resolve-tool.sh":
            found.append(rel)
            continue
        # Must actually source the resolver (not merely mention it).
        if SOURCE_RESOLVER_RE.search(text) or rel.endswith(
            "porcelain-bypass/run-selftest.sh"
        ):
            found.append(rel)
            continue
    return found


def strip_comment(s: str) -> str:
    out = []
    i = 0
    in_sq = in_dq = False
    dollar_brace = 0
    while i < len(s):
        c = s[i]
        if in_sq:
            out.append(c)
            if c == "'":
                in_sq = False
            i += 1
            continue
        if in_dq:
            if c == "\\" and i + 1 < len(s):
                out.append(c)
                out.append(s[i + 1])
                i += 2
                continue
            out.append(c)
            if c == "$" and i + 1 < len(s) and s[i + 1] == "{":
                dollar_brace += 1
            elif c == "}" and dollar_brace:
                dollar_brace -= 1
            elif c == '"':
                in_dq = False
            i += 1
            continue
        if c == "'":
            in_sq = True
            out.append(c)
            i += 1
            continue
        if c == '"':
            in_dq = True
            out.append(c)
            i += 1
            continue
        if c == "$" and i + 1 < len(s) and s[i + 1] == "{":
            dollar_brace += 1
            out.append(c)
            i += 1
            continue
        if c == "}" and dollar_brace:
            dollar_brace -= 1
            out.append(c)
            i += 1
            continue
        if c == "#" and dollar_brace == 0:
            break
        out.append(c)
        i += 1
    return "".join(out)


def tokenize(s: str):
    tokens = []
    i = 0
    n = len(s)
    while i < n:
        c = s[i]
        if c.isspace():
            i += 1
            continue
        if c in ";|&":
            if c == "|" and i + 1 < n and s[i + 1] == "|":
                tokens.append(("OP", "||"))
                i += 2
                continue
            if c == "&" and i + 1 < n and s[i + 1] == "&":
                tokens.append(("OP", "&&"))
                i += 2
                continue
            tokens.append(("OP", c))
            i += 1
            continue
        # redirects
        if c in "<>":
            j = i + 1
            if j < n and s[j] in ">&":
                j += 1
            while j < n and s[j].isdigit():
                j += 1
            tokens.append(("REDIR", s[i:j]))
            i = j
            continue
        if c == "`":
            j = i + 1
            while j < n and s[j] != "`":
                if s[j] == "\\" and j + 1 < n:
                    j += 2
                    continue
                j += 1
            tokens.append(("CMD_SUB", s[i + 1 : j]))
            i = j + 1 if j < n else n
            continue
        if c == "$" and i + 1 < n and s[i + 1] == "(":
            # $((arith)) vs $(cmd)
            if i + 2 < n and s[i + 2] == "(":
                depth = 2
                j = i + 3
                while j < n and depth:
                    if s[j] == "(":
                        depth += 1
                    elif s[j] == ")":
                        depth -= 1
                    j += 1
                tokens.append(("ARITH", s[i:j]))
                i = j
                continue
            depth = 1
            j = i + 2
            while j < n and depth:
                if s[j] == "\\" and j + 1 < n:
                    j += 2
                    continue
                if s[j] == "'":
                    j += 1
                    while j < n and s[j] != "'":
                        j += 1
                    j += 1
                    continue
                if s[j] == '"':
                    j += 1
                    while j < n and s[j] != '"':
                        if s[j] == "\\" and j + 1 < n:
                            j += 2
                            continue
                        j += 1
                    j += 1
                    continue
                if s[j] == "(":
                    depth += 1
                elif s[j] == ")":
                    depth -= 1
                j += 1
            tokens.append(("CMD_SUB", s[i + 2 : j - 1]))
            i = j
            continue
        # word
        j = i
        in_sq = in_dq = False
        while j < n:
            cj = s[j]
            if in_sq:
                if cj == "'":
                    in_sq = False
                j += 1
                continue
            if in_dq:
                if cj == "\\" and j + 1 < n:
                    j += 2
                    continue
                if cj == '"':
                    in_dq = False
                j += 1
                continue
            if cj.isspace() or cj in ";|&`<>":
                break
            if cj == "$" and j + 1 < n and s[j + 1] == "(":
                break
            if cj == "'":
                in_sq = True
                j += 1
                continue
            if cj == '"':
                in_dq = True
                j += 1
                continue
            j += 1
        tokens.append(("WORD", s[i:j]))
        i = j
    return tokens


def defined_functions(text: str):
    return set(FUNC_DEF_RE.findall(text))


def is_allowed(tok: str, functions: set) -> bool:
    if tok in BUILTINS or tok in functions:
        return True
    if PINNED_VAR_RE.match(tok) or ABS_PIN_RE.match(tok):
        return True
    return False


def _sq_open(s: str) -> bool:
    """True if single-quote nesting is open at end of s (ignoring $'...' complexity)."""
    open_sq = False
    i = 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s) and not open_sq:
            i += 2
            continue
        if c == "'":
            open_sq = not open_sq
        i += 1
    return open_sq


def scan_command_positions(text: str, functions: set):
    findings = []
    lines = text.splitlines()
    logical = []
    buf = ""
    start = 1
    in_sq = False
    in_heredoc = None  # terminator
    for i, raw in enumerate(lines, 1):
        if in_heredoc is not None:
            if raw == in_heredoc or raw.strip() == in_heredoc:
                in_heredoc = None
            continue  # heredoc body is not shell command text
        code = strip_comment(raw)
        # Detect <<EOF / <<'EOF' / <<"EOF"
        hm = re.search(r"<<[-]?['\"]?(\w+)['\"]?", code)
        if hm and not in_sq:
            # If the heredoc starts on this line, still scan the prefix before <<
            pre = code[: hm.start()]
            if pre.strip():
                if not buf:
                    start = i
                buf += pre
                logical.append((start, buf))
                buf = ""
            in_heredoc = hm.group(1)
            continue
        if not buf:
            start = i
        if in_sq:
            buf += "\n" + code
            if not _sq_open("'" + code):  # continued; track flips from open
                # recount on full buf
                if not _sq_open(buf):
                    in_sq = False
                    logical.append((start, buf))
                    buf = ""
            continue
        if code.rstrip().endswith("\\") and not code.rstrip().endswith("\\\\"):
            buf += code.rstrip()[:-1] + " "
            continue
        buf += code
        if _sq_open(buf):
            in_sq = True
            continue
        logical.append((start, buf))
        buf = ""
    if buf:
        logical.append((start, buf))

    for lineno, chunk in logical:
        findings.extend(_scan_chunk(chunk, lineno, functions))
    return findings


def _scan_chunk(chunk: str, lineno: int, functions: set):
    out = []
    tokens = tokenize(chunk)
    expect_cmd = True
    in_case = False
    expecting_case_pat = False
    i = 0
    while i < len(tokens):
        kind, val = tokens[i]
        if kind == "CMD_SUB":
            out.extend(_scan_chunk(val, lineno, functions))
            expect_cmd = False
            i += 1
            continue
        if kind == "ARITH":
            expect_cmd = False
            i += 1
            continue
        if kind == "REDIR":
            # optional target word follows
            if i + 1 < len(tokens) and tokens[i + 1][0] == "WORD":
                i += 2
            else:
                i += 1
            continue
        if kind == "OP":
            expect_cmd = True
            if val == ";;":
                expecting_case_pat = True
            i += 1
            continue
        # WORD
        if val == "case":
            in_case = True
            expecting_case_pat = False
            expect_cmd = False
            i += 1
            continue
        if val == "in" and in_case:
            expecting_case_pat = True
            expect_cmd = False
            i += 1
            continue
        if val == "esac":
            in_case = False
            expecting_case_pat = False
            expect_cmd = True
            i += 1
            continue
        if expecting_case_pat:
            # skip pattern tokens until ')' WORD ending with )
            if val.endswith(")"):
                expecting_case_pat = False
                expect_cmd = True
            i += 1
            continue
        # function def: name () {  — skip the name as a "command"
        if (
            i + 2 < len(tokens)
            and tokens[i + 1] == ("WORD", "()")
            or (
                i + 1 < len(tokens)
                and kind == "WORD"
                and val.endswith("()")
            )
        ):
            # skip until '{'
            while i < len(tokens) and not (
                tokens[i][0] == "WORD" and tokens[i][1] == "{"
            ):
                i += 1
            if i < len(tokens):
                i += 1
            expect_cmd = True
            continue
        if not expect_cmd:
            i += 1
            continue
        if val in TRANSPARENT:
            i += 1
            while i < len(tokens) and tokens[i][0] == "WORD" and tokens[i][1].startswith("-"):
                i += 1
            continue
        m = ASSIGN_RE.match(val)
        if m is not None:
            # VAR=$(...) may hide a command sub in the value — already tokenized separately
            i += 1
            continue
        # Real command word — only pure identifiers can be ambient externals.
        if IDENT_RE.match(val) and not is_allowed(val, functions):
            out.append((lineno, val))
        expect_cmd = False
        i += 1
    return out


fail = 0
scripts = discover(ROOT)
if not scripts:
    print("FAIL: no guard-related scripts discovered", file=sys.stderr)
    sys.exit(1)

for rel in scripts:
    path = ROOT / rel
    text = path.read_text(encoding="utf-8", errors="replace")
    functions = defined_functions(text)
    if path.name != "guard-resolve-tool.sh":
        if "guard-resolve-tool.sh" not in text and "guard_resolve_tool" not in text:
            print(f"FAIL: {rel} must source/call guard_resolve_tool", file=sys.stderr)
            fail = 1
            continue
    for lineno, tok in scan_command_positions(text, functions):
        if lineno == 1 and tok.startswith("/"):
            continue
        print(
            f"FAIL: {rel}:{lineno} bare external {tok!r} "
            f"(use guard_resolve_tool + pinned $TOOL)",
            file=sys.stderr,
        )
        fail = 1
    code_lines = [ln for ln in text.splitlines() if not ln.lstrip().startswith("#")]
    body = "\n".join(code_lines)
    if "command -v" in body:
        print(f"FAIL: {rel} must not use command -v (ambient lookup)", file=sys.stderr)
        fail = 1
    if re.search(r"\$\{GREP:-", body):
        print(f"FAIL: {rel} must not read GREP= from env", file=sys.stderr)
        fail = 1

if fail:
    sys.exit(1)
print(
    f"OK: {len(scripts)} guard script(s) pin tools via guard_resolve_tool "
    "(command-position; no ambient lookup)"
)
PY
