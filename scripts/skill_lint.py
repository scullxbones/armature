#!/usr/bin/env python3
"""Verify fenced arm commands in skill files against the actual CLI."""

import os
import re
import shlex
import subprocess
import sys
from pathlib import Path

# Shell operator tokens that separate distinct commands within a single
# logical line (compound statements).
_COMPOUND_OPS = {";", "&&", "||", "|"}

# Redirection operator tokens. Each is dropped along with the single token
# immediately following it (the redirect target).
_REDIRECT_OPS = {">", ">>", "<"}


# Known mandatory flags for specific commands. Keep in sync with
# `MarkFlagRequired` calls in cmd/armature/*.go — a Go test
# (cmd/armature/skill_lint_flags_test.go) fails CI if this drifts out of
# sync with the actual Cobra command definitions.
MANDATORY_FLAGS = {
    "claim": ["--worktree"],
    "transition": ["--to"],  # workflow transition (dag transition uses dag-transition key below)
    "dag-transition": ["--issue"],  # dag transition subcommand
    "assign": ["--worker"],
    "accept-citation": ["--rationale"],
    "source-link": ["--source-id"],
    "decompose-revert": ["--plan"],
    "dag-apply": ["--plan"],  # dag apply subcommand
    "dag-context": [],  # no mandatory flags
    "dag-revert": ["--plan"],  # dag revert subcommand
    "dag-summary": [],  # no mandatory flags
    "link": ["--source", "--dep"],
    "unlink": ["--source", "--dep"],
    "decision": ["--topic", "--choice"],
    "merged": ["--issue"],
    "create": ["--title"],
    "context-history": ["--issue"],
    "reparent": ["--issue", "--parent"],
    # Subcommands / dag group (single-word for TestMandatoryFlagsMatchMarkFlagRequired)
    "apply": ["--plan"],
    "context": [],  # no mandatory flags
    "revert": ["--plan"],
    "summary": [],  # no mandatory flags
    # Subcommands / dag group (multi-word for skill-lint)
    "dag apply": ["--plan"],
    "dag context": [],  # no mandatory flags
    "dag revert": ["--plan"],
    "dag transition": ["--issue"],  # dag transition subcommand
    # dag_transition.go's Use is "transition"; this key satisfies the drift
    # test's first-word match without shadowing workflow "transition" (--to).
    # Never matched at lint time: no real command chain is "transition dag".
    "transition dag": ["--issue"],
    "dag summary": [],  # no mandatory flags
    # sources group subcommands
    "sources accept-citation": ["--rationale"],
    "sources link": ["--source-id"],
    "sources add": ["--url", "--type"],
}

# The arm binary to invoke. Configurable via ARM_BIN so CI (and tests) can
# point at a freshly built binary instead of relying on PATH.
ARM_BIN = os.environ.get("ARM_BIN", "arm")


CANONICAL_DOCS = (
    "README.md",
    "docs/getting-started.md",
    "docs/use-cases.md",
    "docs/commands.md",
    "docs/design/architecture.md",
    "docs/design/roles.md",
)


def find_lint_files(root_dir):
    """Find shipped skills and canonical public documentation to lint.

    Documentation is intentionally an allowlist: historical and archived
    material records old CLI surfaces and is not presented as copyable current
    workflow guidance.
    """
    skills_dir = Path(root_dir) / "internal" / "skillsembed" / "skills"
    lint_files = []
    if skills_dir.exists():
        lint_files.extend(path for path in skills_dir.rglob("*.md") if path.is_file())

    root = Path(root_dir)
    lint_files.extend(path for relative_path in CANONICAL_DOCS
                      if (path := root / relative_path).is_file())
    return sorted(lint_files)


def find_skill_files(root_dir):
    """Backward-compatible alias for callers that only know the old name."""
    return find_lint_files(root_dir)


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


def tokenize_shell_line(line):
    """Tokenize a shell line quote-awarely, keeping operators as tokens.

    Uses `shlex.shlex` with `punctuation_chars=True` so multi-character
    shell operators (`;`, `&&`, `||`, `|`, `>`, `>>`, `<`) come through as
    distinct tokens without being split apart by, or splitting apart, quoted
    content. `commenters='#'` strips a trailing unquoted `# comment` for
    free, quote-aware, instead of the previous naive `"#" not in quotes`
    heuristic.

    Returns None if the line contains unterminated quoting shlex can't
    tokenize (rather than raising), so callers can skip it.
    """
    lexer = shlex.shlex(line, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    lexer.commenters = "#"
    try:
        return list(lexer)
    except ValueError:
        return None


def _split_on_compound_ops(tokens):
    """Split a token stream into command groups at compound-statement ops."""
    groups = [[]]
    for tok in tokens:
        if tok in _COMPOUND_OPS:
            groups.append([])
        else:
            groups[-1].append(tok)
    return [g for g in groups if g]


def _find_arm_invocation(tokens):
    """Find where an `arm` invocation starts within a token group.

    Returns the sub-list of tokens starting at `arm`, or None if this group
    isn't an arm invocation (a bare command, e.g. `arm claim ...`) or a
    command substitution / prefix-assignment invocation (e.g.
    `FILES=$(arm ready ...)`).
    """
    if not tokens:
        return None

    if tokens[0] == "arm":
        return list(tokens)

    # Look for `$( arm ...` — a command substitution or prefix-assignment
    # invocation. Require the token immediately before `arm` to be `(` and
    # the token before that to end in `$`, so a bare word that happens to
    # equal "arm" mid-sentence isn't mistaken for an invocation.
    for i, tok in enumerate(tokens):
        if tok != "arm" or i < 2:
            continue
        if tokens[i - 1] == "(" and tokens[i - 2].endswith("$"):
            command_tokens = tokens[i:]
            # Strip trailing close-paren token(s) left over from the
            # substitution syntax, mirroring the previous `.rstrip(")")`.
            while command_tokens and command_tokens[-1] == ")":
                command_tokens = command_tokens[:-1]
            return command_tokens

    return None


def strip_redirects(tokens):
    """Drop redirection operator tokens and their target token.

    Operates on already-tokenized input rather than the raw string, so a
    redirect target like `<old-path>` (three tokens: `<`, `old-path`, `>`)
    can't be mistaken for a redirect immediately followed by another
    redirect — each `_REDIRECT_OPS` token consumes exactly the one token
    after it.
    """
    result = []
    skip_next = False
    for tok in tokens:
        if skip_next:
            skip_next = False
            continue
        if tok in _REDIRECT_OPS:
            skip_next = True
            continue
        result.append(tok)
    return result


_SHELL_SPECIAL_RE = re.compile(r"[\s;&|<>'\"$()#]")


def display_join(tokens):
    """Rejoin tokens into a string that re-tokenizes back to the same tokens.

    A plain `" ".join` would corrupt a token that itself contains whitespace
    or a shell operator character (e.g. a quoted argument value like
    "a;b" or "contains --not-a-real-flag inside a string") by letting it
    re-split or be mistaken for an operator on a later tokenize_shell_line()
    pass. Any token containing such a character is re-quoted; plain operator
    tokens (`>`, `<`, `;`, ...) and ordinary words contain none of these
    characters as a *whole token* match target (they're single/short
    multi-char operators, not embedded inside a longer word) so they round
    trip as bare tokens and remain recognizable operators on re-tokenization.
    """
    parts = []
    for tok in tokens:
        if tok in _COMPOUND_OPS or tok in _REDIRECT_OPS:
            parts.append(tok)
        elif _SHELL_SPECIAL_RE.search(tok):
            parts.append("'" + tok.replace("'", "'\\''") + "'")
        else:
            parts.append(tok)
    return " ".join(parts)


def has_angle_bracket_placeholder(tokens):
    """Detect a `<placeholder>` synopsis token sequence.

    An unquoted `<name>` span tokenizes (via shlex punctuation_chars) as
    three adjacent tokens: `<`, `name`, `>`. That is also, not coincidentally,
    exactly the shell shape of "redirect stdin from a file named `name`,
    then open an ambiguous empty output redirect" — never a sensible real
    command. Must run on the pre-redirect-strip token list.
    """
    for i in range(len(tokens) - 2):
        if tokens[i] == "<" and tokens[i + 2] == ">":
            return True
    return False


def extract_arm_commands(code_block):
    """Extract all arm command invocations from a code block."""
    commands = []
    lines = join_continued_lines(code_block).split("\n")

    for line in lines:
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        tokens = tokenize_shell_line(line)
        if tokens is None:
            continue

        for group in _split_on_compound_ops(tokens):
            arm_tokens = _find_arm_invocation(group)
            if arm_tokens is None:
                continue
            commands.append(display_join(arm_tokens))

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


def parse_command_line(tokens):
    """Split already-tokenized command tokens into subcommands and arguments.

    Example: ["claim", "TASK-01", "--worktree", "/tmp/wt"]
    Returns: (["claim"], ["TASK-01", "--worktree", "/tmp/wt"])

    For nested commands:
    ["sources", "add", "--url", "PATH"]
    Returns: (["sources", "add"], ["--url", "PATH"])

    `tokens` is expected to already have the leading "arm" and any
    redirection tokens/targets removed, and to come from quote-aware
    tokenization (tokenize_shell_line), so no hand-rolled quote tracking is
    needed here.
    """
    subcommands = []
    args = []
    started_args = False

    for tok in tokens:
        if not started_args and not (tok.startswith("-") or tok.startswith("$")):
            # Looks like a subcommand or positional argument. We'll treat
            # subsequent non-flag tokens as arguments once a flag is seen.
            subcommands.append(tok)
        else:
            started_args = True
            args.append(tok)

    return subcommands, args


def extract_flags(args):
    """Extract flags from command arguments.

    Returns a set of flag names (without values).
    Example: ["TASK-01", "--worktree", "/tmp/wt"] -> {"--worktree"}

    A bare "-" is the documented stdin-sentinel positional argument (e.g.
    `arm review record --assessment -`), not a flag. A bare "--"
    end-of-options marker means every token after it is positional, so flag
    scanning stops there.
    """
    flags = set()
    stopped = False
    for arg in args:
        if stopped:
            continue
        if arg == "--":
            stopped = True
            continue
        if arg == "-":
            continue
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

    tokens = tokenize_shell_line(arm_command)
    if tokens is None:
        return False, f"Could not parse command (unterminated quoting) in: {arm_command}"

    # Square brackets in a command synopsis describe an optional argument to
    # the reader; a shell passes them literally. Shipped skill examples are
    # intended to be copyable, so reject this documentation notation rather
    # than silently accepting a command that will not run as shown. No
    # leading-whitespace requirement (unlike the old regex) so a fused
    # flag+bracket token like "--ttl[=N]" is also caught, and the span may
    # cross a token boundary (e.g. "[--ttl 120]" tokenizes as "[--ttl" and
    # "120]"), so this scans the whole command string rather than per-token.
    if re.search(r"\[[^\]\n]+\]", arm_command):
        return False, f"Command uses bracketed synopsis syntax in: {arm_command}"

    # Angle-bracket placeholders (e.g. "<old-path>") must be checked before
    # redirect-stripping, which would otherwise discard the very tokens that
    # make this a placeholder rather than a real redirect.
    if has_angle_bracket_placeholder(tokens):
        return False, f"Command uses angle-bracket synopsis syntax in: {arm_command}"

    tokens = strip_redirects(tokens)
    if tokens and tokens[0] == "arm":
        tokens = tokens[1:]

    subcommands, args = parse_command_line(tokens)

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
        # Some flags (--example, --schema) make mandatory flags optional
        bypass_flags = {"--example", "--schema"}
        if not (flags & bypass_flags):  # If no bypass flags are present
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

    # Find all shipped skill and canonical documentation files.
    skill_files = find_lint_files(root_dir)

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

    print(f"Skill lint passed: validated {len(skill_files)} skill/documentation files")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
