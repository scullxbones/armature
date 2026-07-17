#!/usr/bin/env python3
"""Tests for skill_lint.py's quote-aware command tokenizer.

These exercise the shlex-based tokenization pipeline directly (extraction,
redirect-stripping, placeholder detection, flag extraction) without needing
a real `arm` binary on PATH, complementing the end-to-end Go tests in
cmd/armature/skill_lint_test.go that run the whole script against a built
binary.
"""

import unittest
import tempfile
from pathlib import Path

from skill_lint import (
    extract_arm_commands,
    extract_flags,
    find_lint_files,
    has_angle_bracket_placeholder,
    strip_redirects,
    tokenize_shell_line,
)


class TestFindLintFiles(unittest.TestCase):
    def test_includes_canonical_docs_but_excludes_archived_docs(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            root = Path(tmp_dir)
            (root / "internal/skillsembed/skills/example").mkdir(parents=True)
            (root / "internal/skillsembed/skills/example/SKILL.md").write_text("# skill\n")
            for name in (
                "README.md",
                "docs/getting-started.md",
                "docs/use-cases.md",
                "docs/commands.md",
                "docs/design/architecture.md",
                "docs/design/roles.md",
            ):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("# canonical\n")
            archive = root / "docs/archive"
            archive.mkdir()
            (archive / "old-workflow.md").write_text("```bash\narm removed-command\n```\n")

            found = {path.relative_to(root).as_posix() for path in find_lint_files(root)}

            self.assertEqual(found, {
                "internal/skillsembed/skills/example/SKILL.md",
                "README.md",
                "docs/getting-started.md",
                "docs/use-cases.md",
                "docs/commands.md",
                "docs/design/architecture.md",
                "docs/design/roles.md",
            })


class TestTokenizeShellLine(unittest.TestCase):
    def test_quoted_semicolon_is_not_a_split_point(self):
        tokens = tokenize_shell_line("arm note TASK-01 --msg 'a;b;c'")
        self.assertEqual(
            tokens, ["arm", "note", "TASK-01", "--msg", "a;b;c"]
        )

    def test_quoted_pipe_is_not_a_split_point(self):
        tokens = tokenize_shell_line("arm note TASK-01 --msg 'a|b'")
        self.assertEqual(tokens, ["arm", "note", "TASK-01", "--msg", "a|b"])

    def test_redirect_operators_are_distinct_tokens(self):
        self.assertEqual(
            tokenize_shell_line("arm ready > out.json"),
            ["arm", "ready", ">", "out.json"],
        )
        self.assertEqual(
            tokenize_shell_line("arm ready >> out.json"),
            ["arm", "ready", ">>", "out.json"],
        )

    def test_compound_operators_are_distinct_tokens(self):
        self.assertEqual(
            tokenize_shell_line("arm doctor && arm validate"),
            ["arm", "doctor", "&&", "arm", "validate"],
        )
        self.assertEqual(
            tokenize_shell_line("arm doctor; arm validate"),
            ["arm", "doctor", ";", "arm", "validate"],
        )

    def test_trailing_comment_is_stripped(self):
        self.assertEqual(
            tokenize_shell_line("arm doctor  # check things"),
            ["arm", "doctor"],
        )

    def test_apostrophe_inside_double_quotes_does_not_break_parsing(self):
        tokens = tokenize_shell_line('arm note TASK-01 --msg "it\'s done"')
        self.assertEqual(tokens, ["arm", "note", "TASK-01", "--msg", "it's done"])

    def test_unterminated_quote_returns_none(self):
        self.assertIsNone(tokenize_shell_line("arm note TASK-01 --msg 'oops"))


class TestStripRedirects(unittest.TestCase):
    def test_drops_redirect_operator_and_target(self):
        self.assertEqual(
            strip_redirects(["arm", "ready", ">", "out.json"]),
            ["arm", "ready"],
        )

    def test_only_consumes_operator_plus_one_target_token(self):
        # This is the regression case: a quote-blind regex strip of
        # ">something" would truncate "arm scope-rename <old-path>" far
        # more aggressively than token-based stripping does. Token-based
        # stripping only ever consumes the exact operator token plus the
        # one token immediately after it, regardless of what came before.
        # (In practice this exact <placeholder><placeholder> shape is
        # already rejected upstream by has_angle_bracket_placeholder before
        # strip_redirects ever runs on it -- see validate_command.)
        tokens = ["arm", "scope-rename", "<", "old-path", ">", "<", "new-path", ">"]
        self.assertEqual(strip_redirects(tokens), ["arm", "scope-rename", "new-path"])


class TestAngleBracketPlaceholder(unittest.TestCase):
    def test_detects_placeholder(self):
        tokens = tokenize_shell_line("arm scope-rename <old-path> <new-path>")
        self.assertTrue(has_angle_bracket_placeholder(tokens))

    def test_does_not_flag_plain_command(self):
        tokens = tokenize_shell_line("arm claim TASK-01 --worktree /tmp/wt")
        self.assertFalse(has_angle_bracket_placeholder(tokens))


class TestExtractFlags(unittest.TestCase):
    def test_bare_dash_is_not_a_flag(self):
        self.assertEqual(
            extract_flags(["--assessment", "-"]), {"--assessment"}
        )

    def test_bare_double_dash_stops_flag_scanning(self):
        self.assertEqual(
            extract_flags(["--worktree", "/tmp/wt", "--", "--not-a-flag"]),
            {"--worktree"},
        )

    def test_flag_with_equals_value(self):
        self.assertEqual(extract_flags(["--ttl=60"]), {"--ttl"})


class TestExtractArmCommands(unittest.TestCase):
    def test_semicolon_separated_commands_split_correctly(self):
        commands = extract_arm_commands("arm doctor; arm validate")
        self.assertEqual(commands, ["arm doctor", "arm validate"])

    def test_quoted_semicolon_does_not_split_command(self):
        commands = extract_arm_commands("arm note TASK-01 --msg 'a;b'")
        self.assertEqual(commands, ["arm note TASK-01 --msg 'a;b'"])

    def test_command_substitution_is_extracted(self):
        commands = extract_arm_commands(
            "FILES=$(arm ready --format json | jq -r '.')"
        )
        self.assertEqual(len(commands), 1)
        self.assertIn("arm ready --format json", commands[0])

    def test_prose_mention_of_arm_is_not_extracted(self):
        commands = extract_arm_commands("Run the arm binary, e.g. see docs.")
        self.assertEqual(commands, [])


if __name__ == "__main__":
    unittest.main()
