#!/usr/bin/env python3
"""Tests for check_adr_principles.py."""

import os
import tempfile
import unittest

from check_adr_principles import check_adr_principles


class TestCheckADRPrinciples(unittest.TestCase):
    def _write(self, root, name, body):
        path = os.path.join(root, name)
        with open(path, "w") as f:
            f.write(body)
        return path

    def test_accepts_non_empty_principles_section(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write(
                tmp,
                "0001-example.md",
                "# ADR\n\n## Status\n\nAccepted\n\n## Principles touched\n\nI1, T2\n\n## Context\n\nText\n",
            )
            self.assertEqual(check_adr_principles(tmp), [])

    def test_reports_missing_principles_section(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write(
                tmp,
                "0001-example.md",
                "# ADR\n\n## Status\n\nAccepted\n\n## Context\n\nText\n",
            )
            self.assertEqual(
                check_adr_principles(tmp),
                ["0001-example.md: missing ## Principles touched section"],
            )

    def test_reports_empty_principles_section(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write(
                tmp,
                "0001-example.md",
                "# ADR\n\n## Principles touched\n\n\n## Context\n\nText\n",
            )
            self.assertEqual(
                check_adr_principles(tmp),
                ["0001-example.md: ## Principles touched must be non-empty"],
            )

    def test_checks_template_but_ignores_readme(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write(tmp, "README.md", "# ADR index\n")
            self._write(tmp, "template.md", "# ADR\n\n## Principles touched\n\nnone\n")
            self.assertEqual(check_adr_principles(tmp), [])


if __name__ == "__main__":
    unittest.main()
