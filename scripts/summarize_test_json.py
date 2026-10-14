#!/usr/bin/env python3

"""Summarize `go test -json` output for CI logs.

Prints package elapsed times (slowest first), the slowest individual tests,
and a warning when a package uses more than 75% of TEST_TIMEOUT. Writes the
same report to $GITHUB_STEP_SUMMARY when that file is set.

Exits non-zero when any package failed so `make test` can surface summary
failures even if the shell status plumbing is lost (Windows make/bash).
"""

from __future__ import annotations

import json
import os
import re
import sys
from collections import OrderedDict
from pathlib import Path
from typing import Mapping


SLOWEST_TESTS = 20
WARN_RATIO = 0.75

_DURATION_RE = re.compile(
    r"(?P<value>\d+(?:\.\d+)?)(?P<unit>ns|us|µs|μs|ms|s|m|h)"
)
_DURATION_UNITS = {
    "ns": 1e-9,
    "us": 1e-6,
    "µs": 1e-6,
    "μs": 1e-6,
    "ms": 1e-3,
    "s": 1.0,
    "m": 60.0,
    "h": 3600.0,
}


class Result:
    def __init__(self, exit_code: int, stdout: str) -> None:
        self.exit_code = exit_code
        self.stdout = stdout


def parse_go_duration(text: str) -> float | None:
    """Parse a Go time.ParseDuration string into seconds."""
    s = (text or "").strip()
    if not s:
        return None
    pos = 0
    total = 0.0
    found = False
    for match in _DURATION_RE.finditer(s):
        if match.start() != pos:
            return None
        total += float(match.group("value")) * _DURATION_UNITS[match.group("unit")]
        pos = match.end()
        found = True
    if not found or pos != len(s):
        return None
    return total


def _fmt_secs(elapsed: float) -> str:
    return f"{elapsed:.1f}s"


def _pct_of_timeout(elapsed: float, timeout_secs: float) -> str:
    return f"{round(100.0 * elapsed / timeout_secs):.0f}%"


def summarize(path: Path, env: Mapping[str, str] | None = None) -> Result:
    environ = os.environ if env is None else env
    timeout_raw = (environ.get("TEST_TIMEOUT") or "").strip()
    timeout_secs = parse_go_duration(timeout_raw) if timeout_raw else None

    packages: OrderedDict[str, dict] = OrderedDict()
    failures: list[tuple[str, str]] = []
    test_output: OrderedDict[tuple[str, str], list[str]] = OrderedDict()
    tests: list[tuple[float, str, str]] = []
    extra: list[str] = []

    try:
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
                elapsed = event.get("Elapsed")

                if pkg:
                    rec = packages.setdefault(
                        pkg, {"pass": False, "fail": False, "elapsed": 0.0}
                    )
                    if action == "pass" and not test:
                        rec["pass"] = True
                        if isinstance(elapsed, (int, float)):
                            rec["elapsed"] = float(elapsed)
                    elif action == "fail":
                        rec["fail"] = True
                        if not test and isinstance(elapsed, (int, float)):
                            rec["elapsed"] = float(elapsed)
                    elif action == "skip" and not test:
                        if isinstance(elapsed, (int, float)):
                            rec["elapsed"] = float(elapsed)

                if (
                    test
                    and isinstance(elapsed, (int, float))
                    and action in ("pass", "fail", "skip")
                ):
                    tests.append((float(elapsed), pkg, test))

                if action == "output" and test:
                    test_output.setdefault((pkg, test), []).append(output)
                elif action == "output" and (
                    output.startswith("FAIL\t") or output.startswith("--- FAIL:")
                ):
                    extra.append(output)

                if action == "fail" and test:
                    failures.append((pkg, test))
                    extra.extend(test_output.get((pkg, test), []))
    except OSError as err:
        msg = f"summarize_test_json.py: cannot read {path}: {err}\n"
        sys.stderr.write(msg)
        return Result(2, "")

    passed = sum(1 for v in packages.values() if v["pass"] and not v["fail"])
    failed = sum(1 for v in packages.values() if v["fail"])

    lines: list[str] = []
    lines.extend(extra)

    if failures:
        lines.append("Failures:")
        for pkg, test in failures:
            lines.append(f"  - {pkg}::{test}" if pkg else f"  - {test}")

    failed_pkgs = [pkg for pkg, v in packages.items() if v["fail"]]
    if failed_pkgs and not failures:
        lines.append("Failed packages (no per-test fail events captured):")
        for pkg in failed_pkgs:
            lines.append(f"  - {pkg}")

    ranked_pkgs = sorted(
        packages.items(), key=lambda item: (-item[1]["elapsed"], item[0])
    )
    lines.append("Package timings (slowest first):")
    if ranked_pkgs:
        for pkg, rec in ranked_pkgs:
            lines.append(f"  {_fmt_secs(rec['elapsed']):>8}  {pkg}")
    else:
        lines.append("  (none)")

    ranked_tests = sorted(tests, key=lambda item: (-item[0], item[1], item[2]))
    lines.append("Top 20 slowest tests:")
    if ranked_tests:
        for elapsed, pkg, name in ranked_tests[:SLOWEST_TESTS]:
            label = f"{pkg}::{name}" if pkg else name
            lines.append(f"  {_fmt_secs(elapsed):>8}  {label}")
    else:
        lines.append("  (none)")

    warnings: list[str] = []
    if timeout_secs and timeout_secs > 0:
        for pkg, rec in ranked_pkgs:
            if rec["elapsed"] > WARN_RATIO * timeout_secs:
                pct = _pct_of_timeout(rec["elapsed"], timeout_secs)
                warnings.append(
                    "WARNING: package "
                    f"{pkg} used {_fmt_secs(rec['elapsed'])} "
                    f"({pct} of TEST_TIMEOUT={timeout_raw})"
                )
    lines.extend(warnings)

    lines.append(f"Summary: {passed} packages passed, {failed} packages failed")
    text = "\n".join(lines) + "\n"
    sys.stdout.write(text)

    summary_path = (environ.get("GITHUB_STEP_SUMMARY") or "").strip()
    if summary_path:
        _write_step_summary(
            Path(summary_path),
            ranked_pkgs,
            ranked_tests[:SLOWEST_TESTS],
            warnings,
            timeout_raw,
            timeout_secs,
        )

    return Result(1 if failed or failures else 0, text)


def _write_step_summary(
    path: Path,
    ranked_pkgs: list,
    ranked_tests: list[tuple[float, str, str]],
    warnings: list[str],
    timeout_raw: str,
    timeout_secs: float | None,
) -> None:
    md: list[str] = ["## Test timings", ""]
    md.append("### Packages (slowest first)")
    md.append("")
    md.append("| Elapsed | Package | % of TEST_TIMEOUT |")
    md.append("| --- | --- | --- |")
    if ranked_pkgs:
        for pkg, rec in ranked_pkgs:
            pct = (
                _pct_of_timeout(rec["elapsed"], timeout_secs)
                if timeout_secs and timeout_secs > 0
                else "—"
            )
            md.append(f"| {_fmt_secs(rec['elapsed'])} | `{pkg}` | {pct} |")
    else:
        md.append("| — | (none) | — |")
    md.append("")
    md.append("### Top 20 slowest tests")
    md.append("")
    md.append("| Elapsed | Test |")
    md.append("| --- | --- |")
    if ranked_tests:
        for elapsed, pkg, name in ranked_tests:
            label = f"{pkg}::{name}" if pkg else name
            md.append(f"| {_fmt_secs(elapsed)} | `{label}` |")
    else:
        md.append("| — | (none) |")
    md.append("")
    if warnings:
        md.append("### Timeout warnings")
        md.append("")
        for warning in warnings:
            md.append(f"- {warning}")
        md.append("")
    elif timeout_raw:
        md.append(f"No package exceeded 75% of TEST_TIMEOUT={timeout_raw}.")
        md.append("")
    try:
        with path.open("a", encoding="utf-8") as fh:
            fh.write("\n".join(md))
            if not md[-1].endswith("\n"):
                fh.write("\n")
    except OSError as err:
        print(
            f"summarize_test_json.py: cannot write GITHUB_STEP_SUMMARY: {err}",
            file=sys.stderr,
        )


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: summarize_test_json.py <go-test-json-file>", file=sys.stderr)
        return 2
    return summarize(Path(sys.argv[1])).exit_code


if __name__ == "__main__":
    raise SystemExit(main())
