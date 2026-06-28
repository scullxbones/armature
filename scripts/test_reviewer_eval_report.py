#!/usr/bin/env python3
"""Unit tests for reviewer_eval_report.py

Tests metric computation with deterministic fixtures.
"""

import unittest
import sys
import os

# Add scripts directory to path so we can import reviewer_eval_report
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from reviewer_eval_report import compute_metrics


class TestEvaluatorMetrics(unittest.TestCase):
    """Tests for evaluator metric computation."""

    def setUp(self):
        """Set up test fixtures."""
        # Synthetic cases with known expected values
        self.cases = [
            {
                "id": "case-001",
                "expected_rating": "green",
                "expected_statuses": {"AC1": "satisfied"}
            },
            {
                "id": "case-002",
                "expected_rating": "red",
                "expected_statuses": {"AC1": "not_satisfied"}
            },
            {
                "id": "case-003",
                "expected_rating": "red",
                "expected_statuses": {"AC1": "satisfied", "AC2": "not_satisfied"}
            },
            {
                "id": "case-004",
                "expected_rating": "yellow",
                "expected_statuses": {"AC1": "partially_satisfied"}
            },
            {
                "id": "case-005",
                "expected_rating": "yellow",
                "expected_statuses": {"AC1": "indeterminate"}
            },
            {
                "id": "case-006",
                "expected_rating": "red",
                "expected_statuses": {"AC1": "not_satisfied"}
            },
            {
                "id": "case-007",
                "expected_rating": "green",
                "expected_statuses": {"AC1": "satisfied", "AC2": "satisfied", "AC3": "satisfied"}
            },
            {
                "id": "case-008",
                "expected_rating": "yellow",
                "expected_statuses": {"AC1": "satisfied", "AC2": "partially_satisfied", "AC3": "not_satisfied"}
            }
        ]

        # Synthetic results that perfectly match expected values
        self.perfect_results = [
            {
                "case_id": "case-001",
                "rating": "green",
                "statuses": {"AC1": "satisfied"}
            },
            {
                "case_id": "case-002",
                "rating": "red",
                "statuses": {"AC1": "not_satisfied"}
            },
            {
                "case_id": "case-003",
                "rating": "red",
                "statuses": {"AC1": "satisfied", "AC2": "not_satisfied"}
            },
            {
                "case_id": "case-004",
                "rating": "yellow",
                "statuses": {"AC1": "partially_satisfied"}
            },
            {
                "case_id": "case-005",
                "rating": "yellow",
                "statuses": {"AC1": "indeterminate"}
            },
            {
                "case_id": "case-006",
                "rating": "red",
                "statuses": {"AC1": "not_satisfied"}
            },
            {
                "case_id": "case-007",
                "rating": "green",
                "statuses": {"AC1": "satisfied", "AC2": "satisfied", "AC3": "satisfied"}
            },
            {
                "case_id": "case-008",
                "rating": "yellow",
                "statuses": {"AC1": "satisfied", "AC2": "partially_satisfied", "AC3": "not_satisfied"}
            }
        ]

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
        # Cases expected green/yellow: case-001, case-004, case-005, case-007, case-008 (5 cases)
        # Cases incorrectly rated red: 0
        # false_red_rate: 0/5 = 0.0
        self.assertEqual(metrics['false_red_rate'], 0.0)

    def test_false_green_rate_metric(self):
        """Test false_green_rate metric computation."""
        metrics = compute_metrics(self.cases, self.perfect_results)
        # Cases expected red: case-002, case-003, case-006 (3 cases)
        # Cases incorrectly rated green/yellow: 0
        # false_green_rate: 0/3 = 0.0
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
