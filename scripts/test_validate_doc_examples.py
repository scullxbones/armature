#!/usr/bin/env python3
"""Regression tests for validate_doc_examples.py."""

import importlib.util
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("validate_doc_examples.py")
SPEC = importlib.util.spec_from_file_location("validate_doc_examples", MODULE_PATH)
validate_doc_examples = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validate_doc_examples)


class FindJSONExamplesTest(unittest.TestCase):
    def test_ignores_gitignored_deployed_skill_copies(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            canonical_doc = root / "docs" / "example.md"
            canonical_skill = (
                root / "internal" / "skillsembed" / "skills" / "example" / "SKILL.md"
            )
            stale_deployed_copy = root / ".claude" / "skills" / "example" / "SKILL.md"

            for path in (canonical_doc, canonical_skill, stale_deployed_copy):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("```json artifact_type=plan\n{}\n```\n", encoding="utf-8")

            scanned_paths = {Path(path) for path, *_ in validate_doc_examples.find_json_examples(root)}

            self.assertEqual(scanned_paths, {canonical_doc, canonical_skill})


if __name__ == "__main__":
    unittest.main()
