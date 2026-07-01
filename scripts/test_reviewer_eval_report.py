#!/usr/bin/env python3
"""Unit tests for reviewer_eval_report.py

Tests metric computation with deterministic fixtures.
"""

import unittest
import sys
import os
import json

# Add scripts directory to path so we can import reviewer_eval_report
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from reviewer_eval_report import compute_metrics


class TestEvaluatorMetrics(unittest.TestCase):
    """Tests for evaluator metric computation.

    Test fixtures are loaded from canonical JSON files:
    - cases.json: internal/review/testdata/evals/cases.json
    - reviewer_eval_results.json: scripts/testdata/reviewer_eval_results.json

    This ensures test data is not duplicated and stays synchronized with the
    canonical source of truth.
    """

    def setUp(self):
        """Load test fixtures from canonical JSON files."""
        # Get the absolute path to the fixture files
        script_dir = os.path.dirname(os.path.abspath(__file__))
        repo_root = os.path.dirname(script_dir)

        # Load cases from canonical fixture file
        cases_path = os.path.join(repo_root, "internal/review/testdata/evals/cases.json")
        with open(cases_path, 'r') as f:
            self.cases = json.load(f)

        # Load perfect results from canonical fixture file
        results_path = os.path.join(script_dir, "testdata/reviewer_eval_results.json")
        with open(results_path, 'r') as f:
            self.perfect_results = json.load(f)

    def test_rating_accuracy_metric(self):
        """Test rating_accuracy metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # 8 cases, all ratings correct: 8/8 = 1.0
        self.assertEqual(metrics['rating_accuracy'], 1.0)

    def test_status_accuracy_metric(self):
        """Test status_accuracy metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # Count total criteria:
        # case-001: 1 (AC1)
        # case-002: 1 (AC1)
        # case-003: 2 (AC1, AC2)
        # case-004: 1 (AC1)
        # case-005: 1 (AC1)
        # case-006: 1 (AC1)
        # case-007: 3 (AC1, AC2, AC3)
        # case-008: 3 (AC1, AC2, AC3)
        # Total: 13 criteria, all correct: 13/13 = 1.0
        self.assertEqual(metrics['status_accuracy'], 1.0)

    def test_false_red_rate_metric(self):
        """Test false_red_rate metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # Cases expected green/yellow: case-001, case-004, case-005, case-007 (4 cases)
        # Cases incorrectly rated red: 0
        # false_red_rate: 0/4 = 0.0
        self.assertEqual(metrics['false_red_rate'], 0.0)

    def test_false_green_rate_metric(self):
        """Test false_green_rate metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # Cases expected red: case-002, case-003, case-006, case-008 (4 cases)
        # Cases incorrectly rated green/yellow: 0
        # false_green_rate: 0/4 = 0.0
        self.assertEqual(metrics['false_green_rate'], 0.0)

    def test_partial_detection_rate_metric(self):
        """Test partial_detection_rate metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # Cases with partial_satisfied expected:
        # case-004: AC1 (1 partial)
        # case-008: AC2 (1 partial)
        # Total: 2 expected partial
        # Cases where partial was detected: 2 (case-004, case-008)
        # partial_detection_rate: 2/2 = 1.0
        self.assertEqual(metrics['partial_detection_rate'], 1.0)

    def test_metrics_all_perfect(self):
        """Test that all metrics are perfect with matching results."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        self.assertEqual(metrics['rating_accuracy'], 1.0)
        self.assertEqual(metrics['status_accuracy'], 1.0)
        self.assertEqual(metrics['false_red_rate'], 0.0)
        self.assertEqual(metrics['false_green_rate'], 0.0)
        self.assertEqual(metrics['partial_detection_rate'], 1.0)

    def test_metrics_threshold(self):
        """Test that rating_accuracy >= 0.75 with perfect results."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        self.assertGreaterEqual(metrics['rating_accuracy'], 0.75)


if __name__ == '__main__':
    unittest.main()
