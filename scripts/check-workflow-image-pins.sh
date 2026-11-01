#!/usr/bin/env bash
# Fail if any GitHub workflow docker run / container image lacks @sha256: pin.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

python3 - "$ROOT" <<'PY'
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
problems = []
sha_re = re.compile(r"@sha256:[0-9a-f]{64}")

def check_image_ref(rel, lineno, ref):
    ref = ref.strip().strip("'\"")
    if not ref:
        return
    # Allow only env indirection when the expanded name is also assigned a
    # digest-pinned value in the same workflow file (checked separately).
    if ref.startswith("${{") or ref.startswith("$"):
        problems.append(
            f"{rel}:{lineno}: image {ref!r} must be a literal digest-pinned ref "
            f"(image:tag@sha256:…), not an indirection"
        )
        return
    if sha_re.search(ref):
        return
    if re.fullmatch(r"sha256:[0-9a-f]{64}", ref):
        return
    problems.append(f"{rel}:{lineno}: image {ref!r} missing @sha256:<digest> pin")

for path in sorted(root.glob(".github/workflows/*.yml")) + sorted(
    root.glob(".github/workflows/*.yaml")
):
    rel = path.relative_to(root).as_posix()
    lines = path.read_text().splitlines()
    i = 0
    while i < len(lines):
        raw = lines[i]
        code = raw.split("#", 1)[0]
        m = re.match(r"^(\s*)image:\s*(.+)$", code)
        if m:
            check_image_ref(rel, i + 1, m.group(2).strip())
            i += 1
            continue
        if re.search(r"(^|[;&]|\s)docker\s+run\b", code):
            blob = code.rstrip()
            j = i
            while blob.endswith("\\"):
                blob = blob[:-1] + " "
                j += 1
                if j >= len(lines):
                    break
                blob += lines[j].split("#", 1)[0].rstrip()
            tokens = blob.split()
            try:
                idx = next(k for k, t in enumerate(tokens) if t == "run")
            except StopIteration:
                i += 1
                continue
            k = idx + 1
            img = None
            while k < len(tokens):
                t = tokens[k]
                if t.startswith("-"):
                    if t in {
                        "-v",
                        "--volume",
                        "-w",
                        "--workdir",
                        "-e",
                        "--env",
                        "-u",
                        "--user",
                        "--name",
                        "--entrypoint",
                        "--network",
                        "--platform",
                        "-p",
                        "--publish",
                    }:
                        k += 2
                        continue
                    if "=" in t:
                        k += 1
                        continue
                    k += 1
                    continue
                img = t
                break
            if img:
                check_image_ref(rel, i + 1, img)
            i = j + 1
            continue
        i += 1

if problems:
    print("FAIL: workflow images must be digest-pinned (@sha256:…):", file=sys.stderr)
    for p in problems:
        print(f"  {p}", file=sys.stderr)
    sys.exit(1)
print("OK: workflow docker run / container images are @sha256-pinned")
PY
