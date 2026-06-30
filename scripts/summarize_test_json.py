#!/usr/bin/env python3

import json
import sys
from collections import OrderedDict


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: summarize_test_json.py <go-test-json-file>", file=sys.stderr)
        return 2

    path = sys.argv[1]
    packages = OrderedDict()
    failures = []

    with open(path, "r", encoding="utf-8") as fh:
        for raw in fh:
            raw = raw.strip()
            if not raw:
                continue
            try:
                event = json.loads(raw)
            except json.JSONDecodeError:
                continue

            pkg = event.get("Package", "")
            action = event.get("Action", "")
            test = event.get("Test", "")
            output = event.get("Output", "").rstrip()

            if pkg:
                packages.setdefault(pkg, {"pass": False, "fail": False})
                if action == "pass":
                    packages[pkg]["pass"] = True
                elif action == "fail":
                    packages[pkg]["fail"] = True

            if action == "fail" and test:
                failures.append(f"{pkg}::{test}" if pkg else test)
            elif action == "output" and (output.startswith("FAIL\t") or output.startswith("--- FAIL:")):
                print(output)

    passed = sum(1 for v in packages.values() if v["pass"] and not v["fail"])
    failed = sum(1 for v in packages.values() if v["fail"])

    if failures:
        print("Failures:")
        for item in failures:
            print(f"  - {item}")

    print(f"Summary: {passed} packages passed, {failed} packages failed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
