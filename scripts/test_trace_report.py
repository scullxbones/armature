#!/usr/bin/env python3
"""Tests for trace_report.py."""

import os
import tempfile
import unittest

from trace_report import scan_test_files


class TestScanTestFiles(unittest.TestCase):
    def _make_test_file(self, path, func_name):
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(f"package foo\n\nfunc {func_name}(t *testing.T) {{}}\n")

    def test_finds_req_tagged_tests(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "pkg", "foo_test.go"),
                "TestFoo_REQ_MYREQ001",
            )
            reqs, count = scan_test_files(tmp)
            self.assertIn("MYREQ001", reqs)
            self.assertEqual(len(reqs["MYREQ001"]), 1)
            self.assertEqual(count, 1)

    def test_ignores_untagged_tests(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "pkg", "foo_test.go"),
                "TestFoo",
            )
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 1)

    def test_prunes_dot_directories(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, ".hidden", "foo_test.go"),
                "TestFoo_REQ_HIDDEN001",
            )
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 0)

    def test_prunes_vendor_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "vendor", "foo_test.go"),
                "TestFoo_REQ_VENDOR001",
            )
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 0)

    def test_prunes_testdata_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "pkg", "testdata", "foo_test.go"),
                "TestFoo_REQ_FIXTURE001",
            )
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 0)

    def test_does_not_prune_normal_subdirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "cmd", "foo_test.go"),
                "TestFoo_REQ_CMD001",
            )
            self._make_test_file(
                os.path.join(tmp, "internal", "bar_test.go"),
                "TestBar_REQ_INT001",
            )
            reqs, count = scan_test_files(tmp)
            self.assertIn("CMD001", reqs)
            self.assertIn("INT001", reqs)
            self.assertEqual(count, 2)

    def test_ignores_lowercase_test_functions(self):
        """Go only runs TestXxx where Xxx doesn't start lowercase; verify we match that."""
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "pkg", "foo_test.go"),
                "Testlowercase_REQ_SHOULDNOTCOUNT",
            )
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 1)

    def test_finds_underscore_prefixed_test_functions(self):
        """Test_REQ_X is a valid Go test name (underscore is not lowercase)."""
        with tempfile.TemporaryDirectory() as tmp:
            self._make_test_file(
                os.path.join(tmp, "pkg", "foo_test.go"),
                "Test_REQ_UNDERSCORECASE",
            )
            reqs, count = scan_test_files(tmp)
            self.assertIn("UNDERSCORECASE", reqs)
            self.assertEqual(count, 1)

    def test_ignores_commented_out_functions(self):
        """func in a comment should not be counted as traced."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "pkg", "foo_test.go")
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w") as f:
                f.write("package foo\n\n// func TestFoo_REQ_COMMENTEDOUT(t *testing.T) {}\n")
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 1)

    def test_ignores_functions_without_testing_t(self):
        """Helpers without *testing.T parameter are not runnable tests."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "pkg", "foo_test.go")
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w") as f:
                f.write("package foo\n\nfunc TestFoo_REQ_NOARGS() {}\n")
            reqs, count = scan_test_files(tmp)
            self.assertEqual(reqs, {})
            self.assertEqual(count, 1)


if __name__ == "__main__":
    unittest.main()
