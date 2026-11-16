#!/usr/bin/env python3
"""Tests for skill_lint.py's quote-aware command tokenizer.

These exercise the shlex-based tokenization pipeline directly (extraction,
redirect-stripping, placeholder detection, flag extraction) without needing
a real `arm` binary on PATH, complementing the end-to-end Go tests in
cmd/armature/skill_lint_test.go that run the whole script against a built
binary.
"""

import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

# `python -m unittest scripts/test_skill_lint.py` puts the repo root on
# sys.path, not scripts/.
sys.path.insert(0, str(Path(__file__).resolve().parent))

from skill_lint import (  # noqa: E402
    _VERIFY_UNPINNED_GIT_LINE_RE,
    extract_arm_commands,
    extract_flags,
    find_lint_files,
    find_verify_armature_script_files,
    has_angle_bracket_placeholder,
    has_unquoted_bracketed_synopsis,
    is_verify_armature_path,
    lint_show_cycle_recovery,
    lint_verify_armature_pinning,
    strip_persistent_prefix,
    strip_redirects,
    tokenize_shell_line,
    validate_command,
)

# Historical unpinned-git pattern (bare value was \S+). Legitimate lines must
# still classify the same way. The pieces are concatenated with + so this
# file does not contain another re.compile literal of the py/redos expression
# (adjacent string literals would be one constant). Do not match the CodeQL
# witness with it: 22 repetitions of `"" A=` took ~1.0s locally.
_LEGACY_UNPINNED_GIT_LINE_RE = re.compile(
    r"^\s*(?:env(?:\s+[A-Za-z_][\w]*=(?:\"[^\"]*\"|'[^']*'|"
    + "\\S+"
    + r"))+\s+)?"
    + r"(?:capture\s+\S+\s+)?git (?!-C\b)(?!--git-dir(?:\s|=))(?!init\b)"
)


class TestFindLintFiles(unittest.TestCase):
    def test_includes_canonical_docs_but_excludes_archived_docs(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            root = Path(tmp_dir)
            (root / "internal/skillsembed/skills/example").mkdir(parents=True)
            (root / "internal/skillsembed/skills/example/SKILL.md").write_text("# skill\n")
            (root / ".agents/skills/capturing-dogfood-findings").mkdir(parents=True)
            (root / ".agents/skills/capturing-dogfood-findings/SKILL.md").write_text("# capture\n")
            for name in (
                "README.md",
                "docs/getting-started.md",
                "docs/use-cases.md",
                "docs/commands.md",
                "docs/harness-hook.md",
                "docs/sensitive-environments.md",
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
                ".agents/skills/capturing-dogfood-findings/SKILL.md",
                "README.md",
                "docs/getting-started.md",
                "docs/use-cases.md",
                "docs/commands.md",
                "docs/harness-hook.md",
                "docs/sensitive-environments.md",
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


class TestPersistentPrefixAndQuotedBrackets(unittest.TestCase):
    def test_strip_persistent_prefix_leaves_subcommand(self):
        tokens = tokenize_shell_line(
            "arm --repo '$TARGET' --format agent --non-interactive bootstrap"
        )
        self.assertEqual(strip_persistent_prefix(tokens), ["bootstrap"])

    def test_validate_accepts_persistent_flags_before_subcommand(self):
        valid, error = validate_command(
            "arm --repo '$TARGET' --format agent --non-interactive doctor",
            {"doctor"},
            {"doctor": {"--strict"}},
        )
        self.assertTrue(valid, error)

    def test_quoted_json_is_not_synopsis_brackets(self):
        command = (
            "arm create --title T --acceptance '[{\"type\":\"test_passes\"}]'"
        )
        self.assertFalse(has_unquoted_bracketed_synopsis(command))
        valid, error = validate_command(
            command,
            {"create"},
            {"create": {"--title", "--acceptance"}},
        )
        self.assertTrue(valid, error)

    def test_unquoted_synopsis_brackets_still_fail(self):
        self.assertTrue(has_unquoted_bracketed_synopsis("arm claim ID --ttl [N]"))


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
        tokens = tokenize_shell_line("arm claim TASK-01 --worktree")
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


class TestClaimWorktreeSyntax(unittest.TestCase):
    def setUp(self):
        self.valid_subcommands = {"claim"}
        self.valid_flags = {"claim": {"--worktree", "--from"}}

    def test_accepts_explicit_destination_with_from(self):
        valid, error = validate_command(
            "arm claim --worktree /tmp/new-task-worktree --from /tmp/story-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertTrue(valid, error)

    def test_accepts_explicit_destination_without_from(self):
        valid, error = validate_command(
            "arm claim --worktree /tmp/new-task-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertTrue(valid, error)

    def test_accepts_canonical_worktree_without_from(self):
        valid, error = validate_command(
            "arm claim --worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertTrue(valid, error)

    def test_accepts_equals_destination_without_from(self):
        valid, error = validate_command(
            "arm claim --worktree=/tmp/new-task-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertTrue(valid, error)

    def test_accepts_equals_destination_with_from(self):
        valid, error = validate_command(
            "arm claim --worktree=/tmp/new-task-worktree --from /tmp/story-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertTrue(valid, error)

    def test_rejects_value_taking_worktree_with_missing_from_value(self):
        valid, error = validate_command(
            "arm claim --worktree /tmp/new-task-worktree --from",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("nonempty --from source", error)

    def test_rejects_valueless_worktree_with_from(self):
        valid, error = validate_command(
            "arm claim TASK --worktree --from /tmp/story-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("explicit --worktree", error)

    def test_rejects_empty_equals_worktree_with_from(self):
        valid, error = validate_command(
            "arm claim TASK --worktree= --from /tmp/story-worktree",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("explicit --worktree", error)

    def test_rejects_empty_spaced_worktree_with_from(self):
        valid, error = validate_command(
            'arm claim TASK --worktree "" --from /tmp/story-worktree',
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("explicit --worktree", error)

    def test_rejects_empty_spaced_from_with_worktree(self):
        valid, error = validate_command(
            'arm claim TASK --worktree /tmp/new-task-worktree --from ""',
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("nonempty --from source", error)

    def test_rejects_empty_equals_from_with_worktree(self):
        valid, error = validate_command(
            "arm claim TASK --worktree /tmp/new-task-worktree --from=",
            self.valid_subcommands,
            self.valid_flags,
        )

        self.assertFalse(valid)
        self.assertIn("nonempty --from source", error)

    def test_accepts_valued_worktree_outside_claim(self):
        valid, error = validate_command(
            "arm ready --worktree /tmp/new-task-worktree",
            {"ready"},
            {"ready": {"--worktree"}},
        )

        self.assertTrue(valid, error)


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


class TestShowCycleRecoveryParser(unittest.TestCase):
    def test_rejects_top_level_notes_parser(self):
        block = """CYCLE=$(arm show "$TASK_ID" --format json | jq -r '
     [.notes // [] | .[] | strings
       | capture("a\\\\.2 cycle (?<n>[0-9]+)/3")?
       | select(.) | .n | tonumber]
     | if length == 0 then 0 else max end
   ')"""
        errors = lint_show_cycle_recovery(block, "SKILL.md", 0)
        self.assertTrue(errors)
        self.assertIn(".issues[0].notes", errors[0])
        self.assertIn("top-level .notes parser", errors[0])

    def test_accepts_envelope_notes_parser(self):
        block = """CYCLE=$(arm show "$TASK_ID" --format json | jq -r '
     [.issues[0].notes // [] | .[] | strings
       | capture("a\\\\.2 cycle (?<n>[0-9]+)/3")?
       | select(.) | .n | tonumber]
     | if length == 0 then 0 else max end
   ')"""
        errors = lint_show_cycle_recovery(block, "SKILL.md", 0)
        self.assertEqual(errors, [])

    def test_ignores_show_without_jq(self):
        errors = lint_show_cycle_recovery(
            'CLAIMED_BY=$(arm show "$TASK_ID" --field claimed_by)',
            "SKILL.md",
            0,
        )
        self.assertEqual(errors, [])


class TestVerifyArmaturePinning(unittest.TestCase):
    def test_path_detection(self):
        self.assertTrue(
            is_verify_armature_path(".agents/skills/verify-armature/SKILL.md")
        )
        self.assertTrue(
            is_verify_armature_path(
                "/repo/.agents/skills/verify-armature/features/worker-init.md"
            )
        )
        self.assertFalse(
            is_verify_armature_path(".agents/skills/capturing-dogfood-findings/SKILL.md")
        )
        self.assertFalse(
            is_verify_armature_path("internal/skillsembed/skills/armature-worker/SKILL.md")
        )

    def test_rejects_unpinned_git_config_backtick(self):
        content = (
            'To clear it inside a temp repo, '
            '`git config --worktree --unset armature.worker-id`.\n'
        )
        errors = lint_verify_armature_pinning(content, "SKILL.md")
        self.assertTrue(errors)
        self.assertIn("unpinned git", errors[0])
        self.assertIn("git config --worktree --unset", errors[0])

    def test_accepts_pinned_git_config_backtick(self):
        content = (
            'To clear it inside a temp repo, '
            '`git -C "$TARGET" config --worktree --unset armature.worker-id`.\n'
        )
        self.assertEqual(lint_verify_armature_pinning(content, "SKILL.md"), [])

    def test_rejects_unpinned_arm_in_fence(self):
        content = "```bash\narm worker-init --check\n```\n"
        errors = lint_verify_armature_pinning(content, "features/worker-init.md")
        self.assertTrue(errors)
        self.assertIn("unpinned arm", errors[0])

    def test_accepts_pinned_arm_and_version(self):
        content = (
            "```bash\n"
            'arm --repo "$TARGET" worker-init --check\n'
            '"./bin/arm" version\n'
            "arm version --format human\n"
            "```\n"
        )
        # ./bin/arm is not matched by the arm-invoke regex (literal arm / $ARM only).
        # bare `arm version` is allowlisted.
        self.assertEqual(
            lint_verify_armature_pinning(content, "SKILL.md"),
            [],
        )

    def test_rejects_unpinned_git_in_script_line(self):
        content = 'git config --worktree --unset armature.worker-id\n'
        errors = lint_verify_armature_pinning(
            content, ".agents/skills/verify-armature/scripts/arm-verify.sh"
        )
        self.assertTrue(errors)
        self.assertIn("unpinned git", errors[0])

    def test_accepts_pinned_script_git_and_ignores_comments(self):
        content = (
            "# git config armature.ops-worktree-path\n"
            'git -C "$TARGET_REPO" config --get armature.worker-id\n'
            'git init -q --bare "$origin"\n'
            'git --git-dir="$origin" rev-parse refs/heads/_armature\n'
        )
        self.assertEqual(
            lint_verify_armature_pinning(
                content, ".agents/skills/verify-armature/scripts/arm-verify.sh"
            ),
            [],
        )

    def test_legacy_and_current_agree_on_legitimate_git_lines(self):
        # (line, matches unpinned git). Pinned -C / --git-dir / init stay
        # allowed. Env-prefixed and capture-prefixed lines stay flagged when
        # the git invocation itself is unpinned.
        cases = (
            ("env FOO=bar git config user.name", True),
            ('env FOO="bar baz" git push origin', True),
            ("env FOO='bar baz' git remote -v", True),
            ("env FOO=bar BAZ=qux git worktree list", True),
            ('env A="" git config x', True),
            ("env A='' git config x", True),
            ('env FOO="a b" BAR=\'c\' git remote -v', True),
            ("env FOO=bar capture label git update-ref HEAD", True),
            ('  env PATH="$PATH" git for-each-ref', True),
            ("capture drive/x git config --get x", True),
            ("  git config user.name", True),
            ("git push origin _armature", True),
            ("git worktree list", True),
            ("git status", True),
            ('capture drive/01 git -C "$TARGET" config --get x', False),
            ('capture drive/05d-remote git -C "$TARGET_REPO" remote add origin "$origin"', False),
            ('git -C "$TARGET" config user.name', False),
            ('git -C"$TARGET" config user.name', False),
            ('git --git-dir "$origin" rev-parse HEAD', False),
            ('git --git-dir="$origin" rev-parse HEAD', False),
            ('git init -q --bare "$origin"', False),
            ('git init -q "$TARGET"', False),
            ('git --git-dir "$origin" init', False),
            ('env FOO=bar git -C "$T" config x', False),
            ('env FOO=bar git --git-dir="$o" rev-parse', False),
            ('env FOO=bar git --git-dir "$o" rev-parse', False),
            ('env FOO=bar git init -q "$T"', False),
            ("echo git config", False),
            ("# git config x", False),
            (
                'capture drive/00-seed-commit env "PATH=$(dirname -- "$ARM_BIN"):$PATH" '
                'git -C "$TARGET_REPO" commit -q -m "docs: seed"',
                False,
            ),
        )
        for line, want in cases:
            legacy = _LEGACY_UNPINNED_GIT_LINE_RE.match(line) is not None
            current = _VERIFY_UNPINNED_GIT_LINE_RE.match(line) is not None
            self.assertEqual(legacy, want, f"legacy: {line}")
            self.assertEqual(current, want, f"current: {line}")
            self.assertEqual(legacy, current, line)

    def test_codeql_redos_witness_finishes_under_bound(self):
        # CodeQL py/redos witness: `env A=` plus many `"" A=`.
        # 30 reps of the legacy pattern is exponential; the current pattern
        # must reject it (no git verb) well inside this bound.
        root = Path(__file__).resolve().parents[1]
        witness_reps = 30
        script = r"""
import importlib.util
import sys
from pathlib import Path

root = Path(sys.argv[1])
reps = int(sys.argv[2])
spec = importlib.util.spec_from_file_location(
    "skill_lint_under_test", root / "scripts" / "skill_lint.py"
)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
line = "env A=" + ('"" A=' * reps)
if mod._VERIFY_UNPINNED_GIT_LINE_RE.match(line) is not None:
    sys.exit("witness matched; expected no unpinned git")
"""
        try:
            proc = subprocess.run(
                [sys.executable, "-c", script, str(root), str(witness_reps)],
                timeout=2.0,
                capture_output=True,
                text=True,
                check=False,
            )
        except subprocess.TimeoutExpired:
            self.fail(
                f"py/redos witness ({witness_reps} reps) exceeded 2s; "
                "bare-value alternative still overlaps quotes"
            )
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)

    def test_find_verify_armature_script_files(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            root = Path(tmp_dir)
            scripts = root / ".agents/skills/verify-armature/scripts"
            scripts.mkdir(parents=True)
            (scripts / "arm-verify.sh").write_text("#!/bin/bash\n")
            (scripts / "notes.txt").write_text("not a script\n")
            found = find_verify_armature_script_files(root)
            self.assertEqual([p.name for p in found], ["arm-verify.sh"])


if __name__ == "__main__":
    unittest.main()
