#!/usr/bin/env python3
"""Regression tests for generated planner skill examples."""

import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("embed_examples.py")
SPEC = importlib.util.spec_from_file_location("embed_examples", MODULE_PATH)
embed_examples = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(embed_examples)


class AddAcceptanceFieldsTest(unittest.TestCase):
    def test_makes_generated_non_terminal_issues_valid_and_traceable(self):
        example = {
            "issues": [
                {"id": "STORY-001", "type": "story", "scope": ""},
                {
                    "id": "TASK-001",
                    "title": "Implement login endpoint",
                    "type": "task",
                    "scope": "",
                },
            ]
        }

        actual = embed_examples.add_acceptance_fields(example)

        for issue in actual["issues"][1:]:
            self.assertTrue(issue["scope"])
            self.assertTrue(issue["priority"])
            self.assertTrue(issue["dod"])
        self.assertIn(
            "TestImplementLoginEndpoint_REQ_TASK_001 passes",
            actual["issues"][1]["acceptance"],
        )
        self.assertEqual(actual["issues"][1]["scope"], "internal/auth/login.go (new)")


if __name__ == "__main__":
    unittest.main()
