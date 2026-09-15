"""Tests for conservative CI change classification."""
import importlib.util
from pathlib import Path
import unittest
import os
import tempfile
from unittest.mock import patch
import subprocess

spec = importlib.util.spec_from_file_location("changes", Path(__file__).with_name("ci-changes.py"))
changes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(changes)


class ChangeTests(unittest.TestCase):
    def test_prose(self):
        self.assertTrue(changes.prose_only(["README.md", "AGENTS.md", "docs/guide.md", "scripts/p6-smoke.md"]))

    def test_runtime_and_unknown_files(self):
        for path in ["docs/api/swagger.json", "api/docs/docs.go", "go.sum", ".env.example", ".github/workflows/ci.yaml", "internal/example.md", "scripts/ci-changes.py"]:
            with self.subTest(path=path):
                self.assertFalse(changes.prose_only(["README.md", path]))

    def test_event_decisions(self):
        cases = [
            ("push", "abc", b"README.md\0", "false"),
            ("pull_request", "abc", b"README.md\0internal/main.go\0", "true"),
            ("workflow_dispatch", "abc", b"README.md\0", "true"),
            ("push", "0" * 40, b"README.md\0", "true"),
        ]
        for event, base, diff, expected in cases:
            with self.subTest(event=event, base=base), tempfile.NamedTemporaryFile() as output:
                with patch.dict(os.environ, {"EVENT_NAME": event, "BASE_SHA": base, "HEAD_SHA": "def", "GITHUB_OUTPUT": output.name}), patch.object(changes.subprocess, "check_output", return_value=diff):
                    changes.main()
                self.assertEqual(Path(output.name).read_text(), f"run={expected}\n")

    def test_unknown_history_runs(self):
        with tempfile.NamedTemporaryFile() as output:
            with patch.dict(os.environ, {"EVENT_NAME": "push", "BASE_SHA": "missing", "GITHUB_OUTPUT": output.name}), patch.object(changes.subprocess, "check_output", side_effect=subprocess.CalledProcessError(128, "git")):
                changes.main()
            self.assertEqual(Path(output.name).read_text(), "run=true\n")

    def test_empty(self):
        self.assertFalse(changes.prose_only([]))


if __name__ == "__main__":
    unittest.main()
