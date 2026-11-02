#!/usr/bin/env bash
# Static check: production guard scripts must not bare-call tools.
# Every shell-out goes through guard_resolve_tool → "$TOOL" absolute path.
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

python3 - "$ROOT" <<'PY'
import pathlib, re, sys

GUARDS = [
    "scripts/check-porcelain-failclosed.sh",
    "scripts/check-no-engine-bin-env.sh",
    "scripts/list-guard-scan-files.sh",
]
TOOLS = {
    "grep", "awk", "sed", "find", "sort", "mktemp", "cp", "rm", "bash",
    "cat", "tr", "mkdir", "chmod", "dirname", "head", "tail", "cut",
}
CMD_PREFIX = re.compile(r"(?:^|[;|&`(]|\$\()\s*([A-Za-z0-9._-]+)")
DOLLAR_TOOL = re.compile(r'"\$[A-Z][A-Z0-9_]*"')
ABS_TOOL = re.compile(r"/(?:usr/)?bin/[A-Za-z0-9._-]+")

fail = 0
for rel in GUARDS:
    path = pathlib.Path(rel)
    text = path.read_text()
    if "guard-resolve-tool.sh" not in text or "guard_resolve_tool" not in text:
        print(f"FAIL: {rel} must source/call guard_resolve_tool", file=sys.stderr)
        fail = 1
        continue
    for i, raw in enumerate(text.splitlines(), 1):
        line = raw.split("#", 1)[0]
        if not line.strip():
            continue
        if "guard_resolve_tool" in line or "guard-resolve-tool.sh" in line:
            continue
        stripped = DOLLAR_TOOL.sub(" ", line)
        stripped = ABS_TOOL.sub(" ", stripped)
        for m in CMD_PREFIX.finditer(stripped):
            tok = m.group(1)
            if tok in TOOLS:
                msg = (
                    f"FAIL: {rel}:{i} bare tool {tok!r} "
                    f"(use guard_resolve_tool + pinned $TOOL): {raw.strip()}"
                )
                print(msg, file=sys.stderr)
                fail = 1
                break
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
print("OK: guard scripts pin tools via guard_resolve_tool (no ambient lookup)")
PY
