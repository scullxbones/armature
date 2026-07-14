#!/usr/bin/env python3
"""Verify fenced arm commands in skill files against the actual CLI."""

import os
import re
import subprocess
import sys
from pathlib import Path


# Known mandatory flags for specific commands. Keep in sync with
# `MarkFlagRequired` calls in cmd/armature/*.go — a Go test
# (cmd/armature/skill_lint_flags_test.go) fails CI if this drifts out of
# sync with the actual Cobra command definitions.
MANDATORY_FLAGS = {
    "claim": ["--worktree"],
    "transition": ["--to"],
    "assign": ["--worker"],
    "accept-citation": ["--rationale"],
    "dag-transition": ["--issue"],
    "link": ["--source", "--dep"],
    "unlink": ["--source", "--dep"],
    "decision": ["--topic", "--choice"],
    "merged": ["--issue"],
    "create": ["--title"],
    "context-history": ["--issue"],
    "decompose-revert": ["--plan"],
    "reparent": ["--issue", "--parent"],
    "source-link": ["--source-id"],
    "sources add": ["--url", "--type"],
}

# The arm binary to invoke. Configurable via ARM_BIN so CI (and tests) can
# point at a freshly built binary instead of relying on PATH.
ARM_BIN = os.environ.get("ARM_BIN", "arm")


def find_skill_files(root_dir):
    """Find every shipped Markdown file in the embedded skills directory."""
    skills_dir = Path(root_dir) / "internal" / "skillsembed" / "skills"
    if not skills_dir.exists():
        return []

    skill_files = []
    for skill_path in skills_dir.rglob("*.md"):
        if skill_path.is_file():
            skill_files.append(skill_path)
    return sorted(skill_files)


FENCE_RE = re.compile(r"^\s*```(\w*)\s*$")


def extract_code_blocks(content):
    """Extract fenced code blocks whose language is bash, sh, or unspecified.

    Walks the file line by line rather than using a single regex, because a
    regex like ``` ```(?:bash|sh)?\\n(.*?)\\n``` ``` mis-pairs fences: a
    non-bash fenced block (e.g. ```json) earlier in the file doesn't match at
    its own opening fence (its language token isn't bash/sh/empty), so the
    regex engine instead matches starting at that block's *closing* fence
    (read as an opening fence with an empty language token), swallowing the
    next block's real opening fence as content. That silently drops bash
    blocks that follow a non-bash block from extraction.
    """
    blocks = []
    lines = content.split("\n")
    in_block = False
    block_lang = None
    current_lines = []

    for line in lines:
        if not in_block:
            m = FENCE_RE.match(line)
            if m:
                in_block = True
                block_lang = m.group(1)
                current_lines = []
        else:
            if line.strip() == "```":
                if block_lang in ("", "bash", "sh"):
                    blocks.append("\n".join(current_lines))
                in_block = False
                block_lang = None
                current_lines = []
            else:
                current_lines.append(line)

    return blocks


def join_continued_lines(code_block):
    """Join shell lines continued by an unquoted trailing backslash."""
    return re.sub(r"\\\n[ \t]*", " ", code_block)


def extract_arm_commands(code_block):
    """Extract all arm command invocations from a code block."""
    commands = []
    lines = join_continued_lines(code_block).split("\n")

    for line in lines:
        # Strip trailing (unquoted) "#" comments before parsing. Skill docs
        # commonly annotate commands, e.g. "arm claim TASK-01 --worktree /tmp/wt  # claim an issue with worktree"
        # and leaving the comment in would corrupt the parsed subcommand chain.
        if "#" in line and '"' not in line and "'" not in line:
            line = line.split("#", 1)[0]

        # Skip comments and empty lines
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        # Handle inline arm commands in a line with pipes, &&, or ;
        # Split on && , | , || , and ; to handle compound statements
        parts = re.split(r"(?:&&|\||\|\||;)", line)

        for part in parts:
            part = part.strip()
            # Strip a trailing (unquoted) shell redirection, e.g.
            # "arm ready > out.json" or "arm ready >> out.json", so the
            # redirect operator and target filename don't leak into the
            # parsed arm command as bogus subcommands/args.
            part = re.sub(r"\s*>>?\s*\S+$", "", part)
            # `arm` can be the command in a command substitution or a prefix
            # assignment, e.g. `FILES=$(arm ready --format json | ...)`.
            # Keep the surrounding shell segment boundary above, then extract
            # the invocation rather than requiring it to start the segment.
            if part.startswith("arm "):
                commands.append(part)
                continue

            # Do not mistake prose such as "arm command, run: ..." for an
            # invocation. Command substitutions have a distinct shell marker.
            match = re.search(r"\$\(\s*(arm(?:\s+[^|&;\n)]*)?)(?=[\s)]|$)", part)
            if match:
                commands.append(match.group(1).rstrip(")"))

    return commands


def get_valid_subcommands():
    """Get the list of valid subcommands from arm --help."""
    try:
        result = subprocess.run(
            [ARM_BIN, "--help"],
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode != 0:
            print(f"ERROR: Failed to get arm help: {result.stderr}", file=sys.stderr)
            return None

        # Parse subcommands from help output
        # The help output has sections like "Workflow Commands:", "DAG Commands:", etc.
        subcommands = set()
        lines = result.stdout.split("\n")
        in_command_section = False

        for line in lines:
            if line.endswith("Commands:"):
                in_command_section = True
                continue

            if in_command_section:
                if line.strip() == "":
                    continue
                if line.startswith("  "):
                    # This is a command line: "  command-name   description"
                    parts = line.strip().split()
                    if parts:
                        subcommands.add(parts[0])
                elif not line.startswith(" "):
                    # We've moved out of the command section
                    in_command_section = False

        return subcommands
    except Exception as e:
        print(f"ERROR: Failed to get valid subcommands: {e}", file=sys.stderr)
        return None


def get_subcommand_help(subcommands):
    """Get the help text for a specific subcommand or subcommand chain.

    Args:
        subcommands: List of subcommand parts, e.g., ["sources", "add"]

    Returns:
        Help text if found, None otherwise
    """
    try:
        cmd = [ARM_BIN] + subcommands + ["--help"]
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=5
        )
        if result.returncode == 0:
            return result.stdout
        return None
    except Exception:
        return None


def extract_valid_flags_from_help(help_text):
    """Extract all valid flag names from help text.

    Looks for lines like:
      --flag string      description
      -f, --flag         description
    """
    if not help_text:
        return set()

    flags = set()
    lines = help_text.split("\n")

    for line in lines:
        # Look for flag definitions: start with whitespace then -- or -
        if not line.startswith("  "):
            continue

        # Remove leading whitespace
        line_stripped = line.lstrip()

        # Extract flags: could be "--flag", "-f", or "-f, --flag"
        if line_stripped.startswith("--"):
            # Format: --flag [type] description
            parts = line_stripped.split()
            if parts:
                flag = parts[0]
                # Remove any trailing comma
                flag = flag.rstrip(",")
                if flag.startswith("--"):
                    flags.add(flag)
        elif line_stripped.startswith("-") and not line_stripped.startswith("--"):
            # Format: -f, --flag or just -f
            parts = line_stripped.split(",")
            for part in parts:
                part = part.strip().split()[0]
                if part.startswith("-"):
                    flags.add(part)

    return flags


def parse_command_line(arm_command):
    """Parse an arm command line into subcommands and arguments.

    Example: "arm claim TASK-01 --worktree /tmp/wt"
    Returns: (["claim"], ["TASK-01", "--worktree", "/tmp/wt"])

    For nested commands:
    "arm sources add --url PATH"
    Returns: (["sources", "add"], ["--url", "PATH"])
    """
    # Remove leading "arm " if present
    if arm_command.startswith("arm "):
        arm_command = arm_command[4:]

    # Split by whitespace, but be careful with quoted arguments
    parts = []
    current = ""
    in_quotes = False

    for char in arm_command:
        if char == '"' or char == "'":
            in_quotes = not in_quotes
        elif char == " " and not in_quotes:
            if current:
                parts.append(current)
            current = ""
        else:
            current += char

    if current:
        parts.append(current)

    if not parts:
        return [], []

    # Separate subcommands (which don't start with -- or -)
    # from arguments (which do or are values)
    subcommands = []
    args = []
    started_args = False

    for part in parts:
        if not started_args and not (part.startswith("-") or part.startswith("$")):
            # Looks like a subcommand or positional argument
            # We'll treat subsequent non-flag parts as arguments
            subcommands.append(part)
        else:
            started_args = True
            args.append(part)

    return subcommands, args


def extract_flags(args):
    """Extract flags from command arguments.

    Returns a set of flag names (without values).
    Example: ["TASK-01", "--worktree", "/tmp/wt"] -> {"--worktree"}
    """
    flags = set()
    for arg in args:
        if arg.startswith("--"):
            # Handle --flag=value format
            flag_name = arg.split("=")[0]
            flags.add(flag_name)
        elif arg.startswith("-") and not arg[1:].replace("-", "").isdigit():
            # Handle -f format (but not negative numbers)
            flags.add(arg)
    return flags


def validate_command(arm_command, valid_subcommands, valid_flags_cache=None):
    """Validate a single arm command.

    Args:
        arm_command: The arm command string to validate
        valid_subcommands: Set of valid top-level subcommand names
        valid_flags_cache: Dict mapping subcommand -> set of valid flags (for performance)

    Returns: (is_valid, error_message)
    """
    if valid_flags_cache is None:
        valid_flags_cache = {}

    subcommands, args = parse_command_line(arm_command)

    if not subcommands:
        return False, f"Invalid arm command syntax: {arm_command}"

    # Check if first subcommand exists
    first_cmd = subcommands[0]
    if first_cmd not in valid_subcommands:
        return False, f"Unknown subcommand '{first_cmd}' in: {arm_command}"

    # Check for mandatory flags. Look up by the full subcommand chain first
    # (e.g. "sources add"), falling back to the top-level command name, so
    # both flat commands (e.g. "transition") and nested ones (e.g.
    # "sources add") can be covered.
    # `subcommands` may include trailing positional/placeholder tokens (e.g.
    # parse_command_line doesn't know which tokens are real subcommands vs.
    # positional args, so a placeholder like "<replacement-url-or-path>"
    # that doesn't start with "-" ends up appended to subcommands). Try
    # progressively shorter prefixes of the chain so a longer, more specific
    # match (e.g. "sources add") is preferred over the bare top-level
    # command, without requiring an exact match against the full chain
    # including trailing positional tokens.
    mandatory_key = first_cmd
    for prefix_len in range(len(subcommands), 0, -1):
        candidate = " ".join(subcommands[:prefix_len])
        if candidate in MANDATORY_FLAGS:
            mandatory_key = candidate
            break
    if mandatory_key in MANDATORY_FLAGS:
        flags = extract_flags(args)
        missing_flags = []
        for mandatory_flag in MANDATORY_FLAGS[mandatory_key]:
            if mandatory_flag not in flags:
                missing_flags.append(mandatory_flag)

        if missing_flags:
            return False, f"Command '{first_cmd}' missing mandatory flags: {', '.join(missing_flags)} in: {arm_command}"

    # Validate flags for this subcommand chain
    # Use the full subcommand chain as the cache key
    cache_key = " ".join(subcommands) if subcommands else ""

    if cache_key and cache_key not in valid_flags_cache:
        help_text = get_subcommand_help(subcommands)
        valid_flags_cache[cache_key] = extract_valid_flags_from_help(help_text)

    if cache_key in valid_flags_cache:
        valid_flags = valid_flags_cache[cache_key]
        if not valid_flags:
            # get_subcommand_help failed to return usable help text (e.g. the
            # subcommand chain doesn't exist or --help failed). Don't silently
            # skip flag validation — report it as a lint error.
            cmd_name = " ".join(subcommands)
            return False, f"Could not determine valid flags for '{cmd_name}' (help text unavailable) in: {arm_command}"

        used_flags = extract_flags(args)
        invalid_flags = []

        for flag in used_flags:
            # Skip positional arguments and values
            if not flag.startswith("-"):
                continue

            # Handle --flag=value format
            flag_name = flag.split("=")[0]

            # Global flags are always valid
            global_flags = {"--debug", "--format", "--non-interactive", "--repo", "--help", "-h"}
            if flag_name in global_flags:
                continue

            if flag_name not in valid_flags:
                invalid_flags.append(flag_name)

        if invalid_flags:
            cmd_name = " ".join(subcommands)
            return False, f"Command '{cmd_name}' has invalid flags: {', '.join(invalid_flags)} in: {arm_command}"

    return True, None


def lint_skill_file(skill_file_path, valid_subcommands, valid_flags_cache=None):
    """Lint a single skill file.

    Args:
        skill_file_path: Path to the SKILL.md file
        valid_subcommands: Set of valid subcommand names
        valid_flags_cache: Dict mapping subcommand -> set of valid flags (for performance)

    Returns: list of error messages (empty if all valid)
    """
    if valid_flags_cache is None:
        valid_flags_cache = {}

    errors = []

    try:
        with open(skill_file_path, "r", encoding="utf-8") as f:
            content = f.read()
    except Exception as e:
        return [f"ERROR reading {skill_file_path}: {e}"]

    code_blocks = extract_code_blocks(content)

    for block_idx, code_block in enumerate(code_blocks):
        commands = extract_arm_commands(code_block)

        for cmd_idx, command in enumerate(commands):
            is_valid, error_msg = validate_command(command, valid_subcommands, valid_flags_cache)

            if not is_valid:
                error_location = f"{skill_file_path}:block{block_idx}:cmd{cmd_idx}"
                errors.append(f"{error_location}: {error_msg}")

    return errors


def main(argv):
    """Main entry point."""
    # Get the root directory (default to current directory)
    root_dir = argv[1] if len(argv) > 1 else "."

    if not os.path.isdir(root_dir):
        print(f"ERROR: {root_dir} is not a directory", file=sys.stderr)
        return 1

    # Get valid subcommands
    valid_subcommands = get_valid_subcommands()
    if valid_subcommands is None:
        print("ERROR: Cannot determine valid arm subcommands", file=sys.stderr)
        return 1

    # Find all skill files
    skill_files = find_skill_files(root_dir)

    if not skill_files:
        # No skill files found - this is OK, just skip
        return 0

    # Lint each skill file (use a shared cache for performance)
    valid_flags_cache = {}
    all_errors = []
    for skill_file in skill_files:
        errors = lint_skill_file(skill_file, valid_subcommands, valid_flags_cache)
        all_errors.extend(errors)

    # Report results
    if all_errors:
        for error in all_errors:
            print(f"FAIL: {error}", file=sys.stderr)
        return 1

    print(f"Skill lint passed: validated {len(skill_files)} skill files")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
