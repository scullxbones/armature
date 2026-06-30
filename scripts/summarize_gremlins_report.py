#!/usr/bin/env python3

import json
import os
import sys


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: summarize_gremlins_report.py <report.json> <label>", file=sys.stderr)
        return 2

    report_path, label = sys.argv[1], sys.argv[2]
    if not os.path.exists(report_path):
        print(f"{label}: no report written")
        return 0

    with open(report_path, "r", encoding="utf-8") as fh:
        data = json.load(fh)

    print(
        f"{label}: efficacy={data['test_efficacy']:.2f}% "
        f"coverage={data['mutations_coverage']:.2f}% "
        f"killed={data['mutants_killed']} "
        f"lived={data['mutants_lived']} "
        f"not_covered={data['mutants_not_covered']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
