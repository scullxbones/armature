#!/usr/bin/env python3
"""Tests for scripts/summarize_test_json.py timing and warning output."""

from __future__ import annotations

import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("summarize_test_json.py")
SPEC = importlib.util.spec_from_file_location("summarize_test_json", MODULE_PATH)
summarize_test_json = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(summarize_test_json)


def _events(*rows: dict) -> str:
    return "".join(json.dumps(row) + "\n" for row in rows)


class ParseGoDurationTest(unittest.TestCase):
    def test_parses_go_test_timeout_units(self):
        self.assertEqual(summarize_test_json.parse_go_duration("20m"), 20 * 60)
        self.assertEqual(summarize_test_json.parse_go_duration("1h30m"), 90 * 60)
        self.assertEqual(summarize_test_json.parse_go_duration("90s"), 90.0)
        self.assertEqual(summarize_test_json.parse_go_duration("1.5s"), 1.5)
        self.assertIsNone(summarize_test_json.parse_go_duration("bogus"))


class SummarizeTimingsTest(unittest.TestCase):
    def _run(self, body: str, timeout: str = "20m", summary: Path | None = None):
        with tempfile.NamedTemporaryFile(
            "w", encoding="utf-8", suffix=".json", delete=False, newline="\n"
        ) as fh:
            fh.write(body)
            path = fh.name
        env = os.environ.copy()
        env["TEST_TIMEOUT"] = timeout
        if summary is not None:
            env["GITHUB_STEP_SUMMARY"] = str(summary)
        else:
            env.pop("GITHUB_STEP_SUMMARY", None)
        try:
            return summarize_test_json.summarize(Path(path), env)
        finally:
            os.unlink(path)

    def test_prints_packages_slowest_first_and_keeps_summary_line(self):
        body = _events(
            {
                "Action": "pass",
                "Package": "example.com/fast",
                "Elapsed": 1.25,
            },
            {
                "Action": "pass",
                "Package": "example.com/slow",
                "Elapsed": 40.5,
            },
            {
                "Action": "pass",
                "Package": "example.com/slow",
                "Test": "TestSlow",
                "Elapsed": 39.0,
            },
            {
                "Action": "pass",
                "Package": "example.com/fast",
                "Test": "TestFast",
                "Elapsed": 0.4,
            },
        )
        result = self._run(body, timeout="20m")
        self.assertEqual(result.exit_code, 0)
        self.assertIn("Summary: 2 packages passed, 0 packages failed", result.stdout)
        pkg_idx = result.stdout.index("Package timings (slowest first):")
        slow_idx = result.stdout.index("example.com/slow")
        fast_idx = result.stdout.index("example.com/fast")
        self.assertLess(pkg_idx, slow_idx)
        self.assertLess(slow_idx, fast_idx)
        self.assertIn("40.5s", result.stdout)
        self.assertIn("Top 20 slowest tests:", result.stdout)
        test_block = result.stdout.split("Top 20 slowest tests:", 1)[1]
        self.assertLess(test_block.index("TestSlow"), test_block.index("TestFast"))
        self.assertNotIn("WARNING:", result.stdout)

    def test_warns_when_package_exceeds_75_percent_of_timeout(self):
        body = _events(
            {
                "Action": "pass",
                "Package": "example.com/edge",
                "Elapsed": 900.0,
            },
            {
                "Action": "pass",
                "Package": "example.com/hot",
                "Elapsed": 960.0,
            },
        )
        result = self._run(body, timeout="20m")
        self.assertEqual(result.exit_code, 0)
        self.assertIn(
            "WARNING: package example.com/hot used 960.0s (80% of TEST_TIMEOUT=20m)",
            result.stdout,
        )
        warnings = [ln for ln in result.stdout.splitlines() if ln.startswith("WARNING:")]
        self.assertEqual(len(warnings), 1)
        self.assertNotIn("example.com/edge", warnings[0])

    def test_caps_slowest_tests_at_twenty(self):
        rows = []
        for i in range(25):
            rows.append(
                {
                    "Action": "pass",
                    "Package": "example.com/p",
                    "Test": f"TestN{i:02d}",
                    "Elapsed": float(i),
                }
            )
        rows.append({"Action": "pass", "Package": "example.com/p", "Elapsed": 100.0})
        result = self._run(_events(*rows))
        listed = [line for line in result.stdout.splitlines() if "::TestN" in line]
        self.assertEqual(len(listed), 20)
        self.assertIn("TestN24", result.stdout)
        self.assertNotIn("TestN04", result.stdout)

    def test_failed_run_keeps_failure_listing_and_nonzero_exit(self):
        body = _events(
            {
                "Action": "output",
                "Package": "example.com/bad",
                "Test": "TestBoom",
                "Output": "boom\n",
            },
            {
                "Action": "fail",
                "Package": "example.com/bad",
                "Test": "TestBoom",
                "Elapsed": 2.0,
            },
            {
                "Action": "fail",
                "Package": "example.com/bad",
                "Elapsed": 2.5,
            },
        )
        result = self._run(body)
        self.assertEqual(result.exit_code, 1)
        self.assertIn("Failures:", result.stdout)
        self.assertIn("example.com/bad::TestBoom", result.stdout)
        self.assertIn("Summary: 0 packages passed, 1 packages failed", result.stdout)

    def test_writes_github_step_summary(self):
        with tempfile.NamedTemporaryFile(
            "w+", encoding="utf-8", suffix=".md", delete=False
        ) as fh:
            summary = Path(fh.name)
        try:
            body = _events(
                {
                    "Action": "pass",
                    "Package": "example.com/hot",
                    "Elapsed": 960.0,
                },
                {
                    "Action": "pass",
                    "Package": "example.com/hot",
                    "Test": "TestHot",
                    "Elapsed": 800.0,
                },
            )
            result = self._run(body, timeout="20m", summary=summary)
            self.assertEqual(result.exit_code, 0)
            text = summary.read_text(encoding="utf-8")
            self.assertIn("Packages (slowest first)", text)
            self.assertIn("example.com/hot", text)
            self.assertIn("TestHot", text)
            self.assertIn("WARNING:", text)
            self.assertIn("960.0s", result.stdout)
        finally:
            summary.unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
