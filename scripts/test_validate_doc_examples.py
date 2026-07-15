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

    def test_matches_hyphenated_artifact_types(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            doc = root / "docs" / "example.md"
            doc.parent.mkdir(parents=True, exist_ok=True)
            doc.write_text(
                "```json artifact_type=review-bundle\n{}\n```\n", encoding="utf-8"
            )

            examples = validate_doc_examples.find_json_examples(root)

            self.assertEqual(len(examples), 1)
            self.assertEqual(examples[0][2], "review-bundle")


class FormatValidationErrorTest(unittest.TestCase):
    def test_formats_plain_string_error_as_is(self):
        # validate_json_example() returns plain strings (not jsonschema
        # ValidationError objects) for malformed JSON, since the document
        # never reaches schema validation. format_validation_error must
        # handle that shape without raising AttributeError.
        message = "Invalid JSON: Expecting value: line 1 column 1 (char 0)"

        result = validate_doc_examples.format_validation_error(message)

        self.assertEqual(result, message)


if __name__ == "__main__":
    unittest.main()
