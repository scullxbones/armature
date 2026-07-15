#!/usr/bin/env python3
"""
Validate JSON examples in docs and skills against their schemas.

Scans all Markdown files in docs/ and .claude/skills/ for fenced JSON code blocks
that declare an artifact type. Validates each example against the corresponding
JSON Schema under docs/schemas/.

Usage:
    python3 scripts/validate_doc_examples.py [--repo /path/to/repo]

Exit codes:
    0: All examples valid
    1: One or more examples invalid
    2: Schema or file not found
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path


# Map artifact type names to schema file names
SCHEMA_MAP = {
    "plan": "plan.schema.json",
    "review_bundle": "review-bundle.schema.json",
    "review-bundle": "review-bundle.schema.json",
    "conformance_assessment": "conformance-assessment.schema.json",
    "conformance-assessment": "conformance-assessment.schema.json",
    "activity_index": "activity-index.schema.json",
    "activity-index": "activity-index.schema.json",
}

# Try to import jsonschema; provide helpful error if not available
try:
    import jsonschema
    from jsonschema import Draft7Validator
except ImportError:
    print("Error: jsonschema module not found. Install with: pip3 install jsonschema", file=sys.stderr)
    sys.exit(2)


def find_json_examples(repo_root):
    """
    Find all JSON code blocks in Markdown files that explicitly declare an artifact type.
    Returns a list of (file_path, line_number, artifact_type, json_text) tuples.

    Only validates examples that explicitly declare artifact_type=<type> in the fence.
    Examples without explicit declaration are skipped (these are typically placeholder
    examples in specs that show schema structure rather than valid instances).
    """
    examples = []

    # Directories to scan
    scan_dirs = [
        repo_root / "docs",
        repo_root / ".claude" / "skills",
        repo_root / ".gemini" / "skills",
    ]

    for scan_dir in scan_dirs:
        if not scan_dir.exists():
            continue

        for md_file in scan_dir.rglob("*.md"):
            try:
                with open(md_file, "r", encoding="utf-8") as f:
                    content = f.read()
            except Exception as e:
                print(f"Warning: Could not read {md_file}: {e}", file=sys.stderr)
                continue

            # Find JSON code blocks with explicit artifact_type declaration
            # Pattern: ```json artifact_type=<type>
            pattern = r'```json\s+artifact_type=(\w+)\s*\n(.*?)```'

            line_num = 1
            for match in re.finditer(pattern, content, re.DOTALL):
                # Count lines up to this match for accurate line numbers
                before_match = content[:match.start()]
                line_num = before_match.count('\n') + 1

                artifact_type = match.group(1)
                json_text = match.group(2).strip()

                if artifact_type:
                    examples.append((str(md_file), line_num, artifact_type, json_text))

    return examples




def load_schema(schema_path):
    """Load and parse a JSON Schema file."""
    try:
        with open(schema_path, "r", encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        print(f"Error: Schema file not found: {schema_path}", file=sys.stderr)
        return None
    except json.JSONDecodeError as e:
        print(f"Error: Invalid schema JSON in {schema_path}: {e}", file=sys.stderr)
        return None


def validate_json_example(json_text, schema):
    """
    Validate a JSON example against a schema.
    Returns (is_valid, errors_list) tuple.
    """
    try:
        obj = json.loads(json_text)
    except json.JSONDecodeError as e:
        return False, [f"Invalid JSON: {e}"]

    validator = Draft7Validator(schema)
    errors = list(validator.iter_errors(obj))

    if not errors:
        return True, []

    return False, errors


def format_validation_error(error):
    """Format a jsonschema validation error for display."""
    path = " -> ".join(str(p) for p in error.path) if error.path else "<root>"
    return f"{path}: {error.message}"


def main():
    parser = argparse.ArgumentParser(
        description="Validate JSON examples in docs and skills against their schemas"
    )
    parser.add_argument(
        "--repo",
        type=Path,
        default=Path.cwd(),
        help="Repository root directory (default: current directory)",
    )
    args = parser.parse_args()

    repo_root = args.repo.resolve()
    schemas_dir = repo_root / "docs" / "schemas"

    # Verify schemas exist
    if not schemas_dir.exists():
        print(f"Error: Schemas directory not found: {schemas_dir}", file=sys.stderr)
        return 2

    # Find all JSON examples
    examples = find_json_examples(repo_root)

    if not examples:
        print("No JSON examples found in docs or skills", file=sys.stderr)
        return 0

    print(f"Found {len(examples)} JSON examples to validate")

    # Validate each example
    failures = []
    for file_path, line_num, artifact_type, json_text in examples:
        schema_file = SCHEMA_MAP.get(artifact_type)
        if not schema_file:
            failures.append(
                (file_path, line_num, f"Unknown artifact type: {artifact_type}")
            )
            continue

        schema_path = schemas_dir / schema_file
        schema = load_schema(schema_path)
        if schema is None:
            failures.append(
                (file_path, line_num, f"Could not load schema: {schema_file}")
            )
            continue

        is_valid, errors = validate_json_example(json_text, schema)
        if not is_valid:
            error_msgs = [format_validation_error(e) for e in errors]
            failures.append((file_path, line_num, "; ".join(error_msgs)))

    # Report results
    if failures:
        print(f"\n{len(failures)} validation error(s):\n", file=sys.stderr)
        for file_path, line_num, error in failures:
            print(f"{file_path}:{line_num}: {error}", file=sys.stderr)
        return 1

    print(f"✓ All {len(examples)} examples are valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
