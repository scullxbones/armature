#!/usr/bin/env python3
"""
Embed CLI-generated canonical examples into skills at build time.

This script:
1. Runs `arm decompose-apply --example` to get the canonical example
2. Augments it with acceptance fields for each issue
3. Embeds or validates the example in skill SKILL.md files

Usage:
  python3 scripts/embed_examples.py embed    # Generate and embed examples
  python3 scripts/embed_examples.py check    # Validate embedded examples match CLI
"""

import json
import subprocess
import sys
import re
from pathlib import Path


def get_example_from_cli():
    """
    Run `arm decompose-apply --example` and return the parsed JSON.

    Returns:
        dict: Parsed example plan

    Raises:
        subprocess.CalledProcessError: If arm command fails
        json.JSONDecodeError: If output is not valid JSON
    """
    result = subprocess.run(
        ["arm", "decompose-apply", "--example"],
        capture_output=True,
        text=True,
        check=True,
    )
    return json.loads(result.stdout)


def add_acceptance_fields(example):
    """
    Augment example plan with acceptance fields for each issue.

    For story issues, adds basic verification acceptance.
    For task issues, adds test name patterns following _REQ_ convention.

    Args:
        example (dict): The parsed example plan

    Returns:
        dict: Modified example with acceptance fields
    """
    for issue in example.get("issues", []):
        issue_id = issue.get("id", "")
        issue_type = issue.get("type", "")

        if issue_type == "story":
            # Stories are decomposed, so acceptance is about the decomposition plan
            issue["acceptance"] = [
                f"Decomposition plan created for {issue_id}",
                "All child tasks have dod, scope, and acceptance fields",
                "arm validate passes with no errors",
            ]
        else:
            # Tasks need test-based acceptance criteria
            # Use story parent to construct REQ pattern (e.g., TASK-001 -> REQ_TASK_001)
            req_id = issue_id.replace("-", "_")
            issue["acceptance"] = [
                f"Implementation complete per dod",
                f"Test_{req_id} passes",
                "make check green",
            ]

    return example


def read_skill_file(path):
    """
    Read a skill SKILL.md file.

    Args:
        path (Path): Path to SKILL.md

    Returns:
        str: File contents
    """
    with open(path, "r") as f:
        return f.read()


def write_skill_file(path, content):
    """
    Write content to a skill SKILL.md file.

    Args:
        path (Path): Path to SKILL.md
        content (str): File contents to write
    """
    with open(path, "w") as f:
        f.write(content)


def find_example_block(content):
    """
    Find the JSON example code block in a skill file.

    Looks for a markdown code fence with ```json and ```
    that contains a JSON plan (with version, title, issues fields).

    Args:
        content (str): Skill file content

    Returns:
        tuple: (start_pos, end_pos, matched_text) or None if not found
    """
    # Pattern to match ```json ... ``` blocks that contain our example structure
    pattern = r"```json\n(.*?)```"
    matches = list(re.finditer(pattern, content, re.DOTALL))

    if not matches:
        return None

    # Find the match that contains a valid decomposition plan example
    # (has version, title, and issues fields)
    for match in matches:
        try:
            json_text = match.group(1)
            # Try to parse it
            json.loads(json_text)
            # Check if it has the right structure
            data = json.loads(json_text)
            if "version" in data and "title" in data and "issues" in data:
                return match.start(1), match.end(1), json_text
        except json.JSONDecodeError:
            continue

    return None


def embed_example_in_skill(skill_path, example_plan):
    """
    Embed a generated example plan into a skill SKILL.md file.

    Replaces the JSON example block with the generated one.

    Args:
        skill_path (Path): Path to the skill SKILL.md file
        example_plan (dict): The example plan to embed

    Returns:
        bool: True if successful, False if example not found
    """
    content = read_skill_file(skill_path)

    result = find_example_block(content)
    if not result:
        print(f"Warning: No example block found in {skill_path}")
        return False

    start_pos, end_pos, _ = result

    # Generate the new JSON block with proper newline
    json_str = json.dumps(example_plan, indent=2)
    new_block = json_str + "\n"

    # Replace the old block with the new one
    new_content = content[:start_pos] + new_block + content[end_pos:]

    write_skill_file(skill_path, new_content)
    return True


def check_example_drift(skill_path, example_plan):
    """
    Check if the embedded example matches the current CLI example.

    Args:
        skill_path (Path): Path to the skill SKILL.md file
        example_plan (dict): The current example plan from CLI

    Returns:
        bool: True if embedded example matches, False if drifted
    """
    content = read_skill_file(skill_path)

    result = find_example_block(content)
    if not result:
        print(f"Error: No example block found in {skill_path}")
        return False

    _, _, json_text = result

    try:
        embedded_example = json.loads(json_text)
    except json.JSONDecodeError as e:
        print(f"Error: Failed to parse embedded example in {skill_path}: {e}")
        return False

    # Normalize both for comparison (sort keys, etc.)
    cli_json = json.dumps(example_plan, sort_keys=True)
    embedded_json = json.dumps(embedded_example, sort_keys=True)

    if cli_json != embedded_json:
        print(f"Drift detected in {skill_path}")
        print("Current CLI output:")
        print(json.dumps(example_plan, indent=2))
        print("\nEmbedded example:")
        print(json.dumps(embedded_example, indent=2))
        return False

    return True


def main():
    """Main entry point."""
    if len(sys.argv) < 2:
        print("Usage: python3 scripts/embed_examples.py [embed|check]")
        sys.exit(1)

    mode = sys.argv[1]

    if mode not in ("embed", "check"):
        print(f"Error: Unknown mode '{mode}'. Use 'embed' or 'check'")
        sys.exit(1)

    # Get the example from CLI
    try:
        example = get_example_from_cli()
    except subprocess.CalledProcessError as e:
        print(f"Error: Failed to run 'arm decompose-apply --example': {e}")
        print(f"stderr: {e.stderr}")
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Error: Failed to parse 'arm decompose-apply --example' output: {e}")
        sys.exit(1)

    # Add acceptance fields
    example_with_acceptance = add_acceptance_fields(example)

    # Find the planner skill
    skill_path = Path("internal/skillsembed/skills/armature-planner/SKILL.md")

    if not skill_path.exists():
        print(f"Error: Skill file not found: {skill_path}")
        sys.exit(1)

    if mode == "embed":
        print(f"Embedding example into {skill_path}...")
        if embed_example_in_skill(skill_path, example_with_acceptance):
            print("✓ Example embedded successfully")
        else:
            print("✗ Failed to embed example")
            sys.exit(1)
    else:  # check
        print(f"Checking example drift in {skill_path}...")
        if check_example_drift(skill_path, example_with_acceptance):
            print("✓ No drift detected")
        else:
            print("✗ Example drift detected")
            sys.exit(1)


if __name__ == "__main__":
    main()
