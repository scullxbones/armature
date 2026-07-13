#!/usr/bin/env python3
"""Check ADR files for a non-empty Principles touched section."""

import os
import re
import sys


ADR_FILE_RE = re.compile(r"^(?:\d{4}-.+|template)\.md$")


def _adr_files(adr_dir):
    for name in sorted(os.listdir(adr_dir)):
        if ADR_FILE_RE.match(name):
            yield name


def _principles_value(lines):
    for index, line in enumerate(lines):
        if line.strip() != "## Principles touched":
            continue

        values = []
        for next_line in lines[index + 1 :]:
            stripped = next_line.strip()
            if stripped.startswith("#"):
                break
            if stripped:
                values.append(stripped)
        return "\n".join(values)
    return None


def check_adr_principles(adr_dir):
    errors = []
    for name in _adr_files(adr_dir):
        path = os.path.join(adr_dir, name)
        with open(path, encoding="utf-8") as f:
            value = _principles_value(f.readlines())

        if value is None:
            errors.append(f"{name}: missing ## Principles touched section")
        elif not value:
            errors.append(f"{name}: ## Principles touched must be non-empty")
    return errors


def main(argv):
    adr_dir = argv[1] if len(argv) > 1 else "docs/adr"
    errors = check_adr_principles(adr_dir)
    for error in errors:
        print(error, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
