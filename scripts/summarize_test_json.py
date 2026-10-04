#!/usr/bin/env python3

"""Summarize `go test -json` output for CI logs.

Exits non-zero when any package failed so `make test` can surface summary
failures even if the shell status plumbing is lost (Windows make/bash).
"""

from __future__ import annotations

import json
import sys
from collections import OrderedDict
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: summarize_test_json.py <go-test-json-file>", file=sys.stderr)
        return 2

    path = Path(sys.argv[1])
    packages: OrderedDict[str, dict[str, bool]] = OrderedDict()
    failures: list[tuple[str, str]] = []
    test_output: OrderedDict[tuple[str, str], list[str]] = OrderedDict()

    try:
        # newline="" + universal newlines keeps CRLF (Windows go test pipes) tidy.
        with path.open("r", encoding="utf-8", errors="replace", newline="") as fh:
            for raw in fh:
                raw = raw.strip()
                if not raw:
                    continue
                try:
                    event = json.loads(raw)
                except json.JSONDecodeError:
                    continue

                pkg = event.get("Package", "") or ""
                action = event.get("Action", "") or ""
                test = event.get("Test", "") or ""
                output = (event.get("Output", "") or "").rstrip("\n").rstrip("\r")

                if pkg:
                    packages.setdefault(pkg, {"pass": False, "fail": False})
                    if action == "pass" and not test:
                        packages[pkg]["pass"] = True
                    elif action == "fail":
                        # Package-level fail, or any test fail inside the package.
                        packages[pkg]["fail"] = True

                if action == "output" and test:
                    test_output.setdefault((pkg, test), []).append(output)
                elif action == "output" and (
                    output.startswith("FAIL\t") or output.startswith("--- FAIL:")
                ):
                    print(output)

                if action == "fail" and test:
                    failures.append((pkg, test))
                    for line in test_output.get((pkg, test), []):
                        print(line)
    except OSError as err:
        print(f"summarize_test_json.py: cannot read {path}: {err}", file=sys.stderr)
        return 2

    passed = sum(1 for v in packages.values() if v["pass"] and not v["fail"])
    failed = sum(1 for v in packages.values() if v["fail"])

    if failures:
        print("Failures:")
        for pkg, test in failures:
            print(f"  - {pkg}::{test}" if pkg else f"  - {test}")

    print(f"Summary: {passed} packages passed, {failed} packages failed")
    return 1 if failed or failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
