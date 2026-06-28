#!/usr/bin/env python3
"""Evaluator for reviewer metric computation.

Computes 5 metrics by comparing actual results against a labeled corpus:
1. rating_accuracy: fraction of cases where actual rating matches expected
2. status_accuracy: fraction of criterion statuses correct across all cases
3. false_red_rate: proportion of cases expected green/yellow but rated red
4. false_green_rate: proportion of cases expected red but rated green/yellow
5. partial_detection_rate: fraction of partially_satisfied criteria correctly identified

Exits 0 if rating_accuracy >= 0.75, else 1.

Usage: python3 scripts/reviewer_eval_report.py --cases cases.json --results results.json
"""

import json
import sys
import argparse
from collections import defaultdict


def load_json_file(path):
    """Load and parse a JSON file."""
    with open(path, 'r') as f:
        return json.load(f)


def compute_metrics(cases, results):
    """Compute all 5 metrics.

    Args:
        cases: List of case dicts with expected ratings/statuses
        results: List of result dicts with actual ratings/statuses

    Returns:
        Dict with keys: rating_accuracy, status_accuracy, false_red_rate,
                       false_green_rate, partial_detection_rate
    """
    # Build lookup of results by case_id
    results_by_case = {r['case_id']: r for r in results}

    # Metrics tracking
    correct_ratings = 0
    total_ratings = 0
    correct_statuses = 0
    total_statuses = 0

    false_red_count = 0
    false_red_total = 0

    false_green_count = 0
    false_green_total = 0

    partial_correct = 0
    partial_total = 0

    for case in cases:
        case_id = case['id']
        expected_rating = case['expected_rating']
        expected_statuses = case['expected_statuses']

        # Get the result for this case
        if case_id not in results_by_case:
            # Case not in results - treat as incorrect; count each expected status once.
            total_ratings += 1
            total_statuses += len(expected_statuses)
            continue

        result = results_by_case[case_id]
        actual_rating = result['rating']
        actual_statuses = result['statuses']

        # Metric 1: rating_accuracy
        total_ratings += 1
        if actual_rating == expected_rating:
            correct_ratings += 1

        # Metric 3 & 4: false_red_rate and false_green_rate
        if expected_rating in ['green', 'yellow']:
            false_red_total += 1
            if actual_rating == 'red':
                false_red_count += 1

        if expected_rating == 'red':
            false_green_total += 1
            if actual_rating in ['green', 'yellow']:
                false_green_count += 1

        # Metric 2: status_accuracy
        for criterion_id, expected_status in expected_statuses.items():
            total_statuses += 1
            actual_status = actual_statuses.get(criterion_id, 'unknown')
            if actual_status == expected_status:
                correct_statuses += 1

            # Metric 5: partial_detection_rate
            if expected_status == 'partially_satisfied':
                partial_total += 1
                if actual_status == 'partially_satisfied':
                    partial_correct += 1

    # Compute final metrics
    rating_accuracy = correct_ratings / total_ratings if total_ratings > 0 else 0.0
    status_accuracy = correct_statuses / total_statuses if total_statuses > 0 else 0.0
    false_red_rate = false_red_count / false_red_total if false_red_total > 0 else 0.0
    false_green_rate = false_green_count / false_green_total if false_green_total > 0 else 0.0
    partial_detection_rate = partial_correct / partial_total if partial_total > 0 else 0.0

    return {
        'rating_accuracy': rating_accuracy,
        'status_accuracy': status_accuracy,
        'false_red_rate': false_red_rate,
        'false_green_rate': false_green_rate,
        'partial_detection_rate': partial_detection_rate,
        'total_cases': len(cases),
        'total_criteria': total_statuses,
        'correct_ratings': correct_ratings,
        'correct_statuses': correct_statuses,
    }


def format_metrics(metrics):
    """Format metrics for display."""
    output = []
    output.append("=== Evaluator Metrics Report ===")
    output.append(f"Total cases: {metrics['total_cases']}")
    output.append(f"Total criteria: {metrics['total_criteria']}")
    output.append("")
    output.append(f"rating_accuracy:       {metrics['rating_accuracy']:.2%} ({metrics['correct_ratings']}/{metrics['total_cases']})")
    output.append(f"status_accuracy:       {metrics['status_accuracy']:.2%} ({metrics['correct_statuses']}/{metrics['total_criteria']})")
    output.append(f"false_red_rate:        {metrics['false_red_rate']:.2%}")
    output.append(f"false_green_rate:      {metrics['false_green_rate']:.2%}")
    output.append(f"partial_detection_rate: {metrics['partial_detection_rate']:.2%}")
    output.append("")

    if metrics['rating_accuracy'] >= 0.75:
        output.append("✓ rating_accuracy >= 0.75: PASS")
    else:
        output.append("✗ rating_accuracy < 0.75: FAIL")

    return "\n".join(output)


def main():
    parser = argparse.ArgumentParser(
        description='Compute evaluator metrics from results against corpus'
    )
    parser.add_argument('--cases', required=True, help='Path to cases.json corpus')
    parser.add_argument('--results', required=True, help='Path to results.json file')

    args = parser.parse_args()

    try:
        cases = load_json_file(args.cases)
        results = load_json_file(args.results)
    except Exception as e:
        print(f"Error loading files: {e}", file=sys.stderr)
        return 1

    metrics = compute_metrics(cases, results)
    print(format_metrics(metrics))

    # Exit based on rating_accuracy threshold
    if metrics['rating_accuracy'] >= 0.75:
        return 0
    else:
        return 1


if __name__ == '__main__':
    sys.exit(main())
