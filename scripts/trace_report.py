#!/usr/bin/env python3
"""
trace_report.py - Scan Go test files for spec traceability.

Scans *_test.go files for Test*_REQ_* patterns and prints a report
of traced requirements. Exits with 0 if tests are found or tagged,
otherwise prints a message about no tests tagged.
"""

import os
import re
import sys
from pathlib import Path


def scan_test_files(root_dir):
    """
    Scan *_test.go files in root_dir for Test*_REQ_* patterns.

    Returns a dict mapping requirement IDs to lists of test names.
    """
    requirements = {}
    test_file_count = 0

    # Find all *_test.go files
    for root, dirs, files in os.walk(root_dir):
        dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ('vendor', 'testdata')]
        for filename in files:
            if filename.endswith("_test.go"):
                test_file_count += 1
                filepath = os.path.join(root, filename)

                try:
                    with open(filepath, "r", encoding="utf-8") as f:
                        content = f.read()

                    # Strip Go comments to avoid matching commented-out function declarations.
                    # NOTE: // inside string literals is also stripped, but no plausible string
                    # content can form the func Test*_REQ_*(*testing.T) pattern we look for.
                    stripped = re.sub(r'//[^\n]*', '', content)
                    # strip /* ... */ block comments
                    stripped = re.sub(r'/\*.*?\*/', '', stripped, flags=re.DOTALL)

                    # Pattern: TestSomething_REQ_SOMETHING_ID
                    # Go test naming: TestXxx where Xxx doesn't start with lowercase.
                    # Test[A-Z_0-9]\w* covers TestFoo, TestABC; the group is optional to
                    # also cover Test_REQ_X where _REQ_ follows immediately after Test.
                    # Testfoo_REQ_X is excluded because f is not in [A-Z_0-9].
                    # Also require *testing.T parameter to match only actual runnable tests.
                    pattern = r"func\s+(Test(?:[A-Z_0-9]\w*)?_REQ_\w+)\s*\(\s*\w+\s+\*testing\.T"
                    matches = re.findall(pattern, stripped)

                    for test_name in matches:
                        # Extract the requirement ID (everything after _REQ_)
                        req_match = re.search(r"_REQ_(.+)$", test_name)
                        if req_match:
                            req_id = req_match.group(1)
                            if req_id not in requirements:
                                requirements[req_id] = []
                            requirements[req_id].append({
                                "test": test_name,
                                "file": filepath,
                            })
                except (IOError, UnicodeDecodeError) as e:
                    print(f"Warning: Could not read {filepath}: {e}", file=sys.stderr)

    return requirements, test_file_count


def main():
    """Main entry point."""
    if len(sys.argv) < 2:
        root_dir = "."
    else:
        root_dir = sys.argv[1]

    root_dir = os.path.normpath(root_dir)

    if not os.path.isdir(root_dir):
        print(f"Error: '{root_dir}' is not a valid directory", file=sys.stderr)
        sys.exit(1)

    requirements, test_file_count = scan_test_files(root_dir)

    if not requirements:
        print("no tests tagged")
        return 0

    # Print report
    print(f"Traced Requirements Report ({len(requirements)} requirement(s)):")
    print()

    for req_id in sorted(requirements.keys()):
        tests = requirements[req_id]
        print(f"  REQ_{req_id}:")
        for test in tests:
            print(f"    - {test['test']} ({test['file']})")

    return 0


if __name__ == "__main__":
    sys.exit(main())
