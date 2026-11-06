"""Behavioral source selection tests: local Git remotes and fake GitHub only."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import test_release as helpers


class SourceSelectionTests(unittest.TestCase):
    git = helpers.ValidationTests.git

    def setUp(self):
        helpers.ValidationTests.setUp(self)
        # The immutable source has the strict validator but not the new selector.
        scripts = self.repo / "scripts/release"
        scripts.mkdir(parents=True)
        for name in ("validate.py", "validate.sh"):
            shutil.copyfile(HERE / name, scripts / name)
        self.git("add", ".")
        self.git("commit", "-m", "tagged source validator")
        self.sha = self.git("rev-parse", "HEAD")
        self.git("tag", "-d", "v1.2.3")
        self.git("tag", "-a", "v1.2.3", "-m", "immutable release source")
        self.tag_object = self.git("rev-parse", "refs/tags/v1.2.3")
        # A newer trusted workflow must execute the tagged validator, not this
        # deliberately different main revision's validator.
        (scripts / "validate.sh").write_text("#!/usr/bin/env bash\necho wrong-workflow-source >&2\nexit 42\n")
        self.git("add", ".")
        self.git("commit", "-m", "newer workflow definition")
        self.workflow_sha = self.git("rev-parse", "HEAD")
        self.origin = self.root / "origin.git"
        subprocess.run(["git", "clone", "--bare", str(self.repo), str(self.origin)], check=True, capture_output=True)
        self.git("remote", "set-url", "origin", str(self.origin))
        self.git("checkout", "--detach", self.workflow_sha)
        self.outputs = self.root / "actions-output"
        self.outputs.write_text("preserved=existing-output\n")
        self.commands = self.root / "git-commands.jsonl"
        real_git = shutil.which("git")
        wrapper = "#!" + sys.executable + " -B\nimport json, os, sys\nwith open(" + repr(str(self.commands)) + ", 'a') as output: output.write(json.dumps(sys.argv[1:]) + '\\n')\nos.execv(" + repr(real_git) + ", [" + repr(real_git) + "] + sys.argv[1:])\n"
        (self.fake / "git").write_text(wrapper)
        (self.fake / "git").chmod(0o755)
        runs = json.loads(self.env["RUNS"])
        runs[0]["head_sha"] = self.sha
        self.env.update({"GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_REF_TYPE": "branch", "GITHUB_REF_NAME": "main", "GITHUB_REF": "refs/heads/main", "GITHUB_SHA": self.workflow_sha, "GITHUB_OUTPUT": str(self.outputs), "RELEASE_VERSION_INPUT": "v1.2.3", "RUNS": json.dumps(runs)})

    def select(self, **environment):
        return subprocess.run([sys.executable, "-B", str(HERE / "select_source.py")], cwd=self.repo, env={**self.env, **environment}, capture_output=True, text=True)

    def output_values(self):
        return dict(line.split("=", 1) for line in self.outputs.read_text().splitlines())

    def assert_source(self):
        self.assertEqual(self.git("rev-parse", "HEAD"), self.sha)
        self.assertEqual(self.git("cat-file", "-t", "refs/tags/v1.2.3"), "tag")
        self.assertEqual(self.git("rev-parse", "refs/tags/v1.2.3"), self.tag_object)
        remote = subprocess.check_output(["git", "-C", str(self.origin), "rev-parse", "refs/tags/v1.2.3"], text=True).strip()
        self.assertEqual(remote, self.tag_object)
        values = self.output_values()
        self.assertEqual(values["version"], "v1.2.3")
        self.assertEqual(values["commit"], self.sha)
        self.assertEqual(values["build_time"], self.git("show", "-s", "--format=%cI", self.sha))
        self.assertEqual(values["preserved"], "existing-output")

    def test_main_dispatch_uses_old_tagged_source_and_its_exact_main_ci(self):
        self.assertNotEqual(self.workflow_sha, self.sha)
        result = self.select()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_source()
        # The selected child identity does not mutate the workflow's identity.
        self.assertEqual(self.env["GITHUB_SHA"], self.workflow_sha)
        self.assertEqual(self.env["GITHUB_REF_NAME"], "main")

    def test_missing_local_tag_is_fetched_from_existing_remote_without_moving_it(self):
        self.git("tag", "-d", "v1.2.3")
        result = self.select()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_source()

    def test_raw_commit_checkout_preserves_annotation_and_push_cannot_override_source(self):
        self.git("checkout", "--detach", self.sha)
        result = self.select(GITHUB_EVENT_NAME="push", GITHUB_REF_TYPE="tag", GITHUB_REF_NAME="v1.2.3", GITHUB_REF="refs/tags/v1.2.3", GITHUB_SHA=self.sha, RELEASE_VERSION_INPUT="v9.9.9")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assert_source()

    def test_automatic_event_sha_mismatch_is_rejected_without_checkout(self):
        result = self.select(GITHUB_EVENT_NAME="push", GITHUB_REF_TYPE="tag", GITHUB_REF_NAME="v1.2.3", GITHUB_REF="refs/tags/v1.2.3")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.workflow_sha)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})

    def test_initial_checkout_must_match_trusted_workflow_sha(self):
        result = self.select(GITHUB_SHA=self.sha)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.workflow_sha)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})

    def test_checkout_tag_fallback_clobber_is_rejected_not_silently_repaired(self):
        # Reproduce Actions' problematic fallback, only in this disposable clone.
        self.git("fetch", "--no-tags", "origin", "+" + self.sha + ":refs/tags/v1.2.3")
        self.assertEqual(self.git("cat-file", "-t", "refs/tags/v1.2.3"), "commit")
        self.git("checkout", "--detach", self.sha)
        environment = {**self.env, "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v1.2.3", "GITHUB_SHA": self.sha}
        result = subprocess.run(["bash", "scripts/release/validate.sh"], cwd=self.repo, env=environment, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("release tag must be annotated", result.stderr)
        self.git("checkout", "--detach", self.workflow_sha)
        self.assertNotEqual(self.select().returncode, 0)
        self.assertEqual(self.git("rev-parse", "refs/tags/v1.2.3"), self.sha)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})

    def test_manual_branch_and_invalid_versions_fail_before_any_git_command(self):
        cases = [
            {"GITHUB_REF": "refs/heads/repair", "GITHUB_REF_NAME": "repair"},
            {"GITHUB_REF": "refs/tags/v1.2.3", "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v1.2.3"},
            {"GITHUB_EVENT_NAME": "pull_request"},
            {"GITHUB_EVENT_NAME": "push"},
            {"GITHUB_SHA": "--untrusted-ref"},
        ]
        cases += [{"RELEASE_VERSION_INPUT": value} for value in ("", "main", self.sha, "refs/tags/v1.2.3", "v1.2.3^{commit}", "--upload-pack=attack", "v1.2.3:refs/heads/main", "v1.2.3\nversion=attack", "v01.2.3", "v1.2.3-rc.1")]
        for environment in cases:
            with self.subTest(environment=environment):
                if self.commands.exists():
                    self.commands.unlink()
                result = self.select(**environment)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.commands.exists(), result.stdout + result.stderr)
                self.assertEqual(self.git("rev-parse", "HEAD"), self.workflow_sha)
                self.assertEqual(self.output_values(), {"preserved": "existing-output"})

    def test_missing_lightweight_or_replaced_remote_tag_is_rejected(self):
        result = self.select(RELEASE_VERSION_INPUT="v9.9.9")
        self.assertNotEqual(result.returncode, 0)
        subprocess.run(["git", "-C", str(self.origin), "tag", "v2.0.0", self.sha], check=True)
        result = self.select(RELEASE_VERSION_INPUT="v2.0.0")
        self.assertNotEqual(result.returncode, 0)
        # Refuse a local/remote tag disagreement rather than force-replacing it.
        subprocess.run(["git", "-C", str(self.origin), "update-ref", "refs/tags/v1.2.3", self.workflow_sha], check=True)
        result = self.select()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.git("rev-parse", "refs/tags/v1.2.3"), self.tag_object)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.workflow_sha)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})

    def test_ci_for_workflow_revision_or_api_failure_cannot_authorize_old_source(self):
        runs = json.loads(self.env["RUNS"])
        runs[0]["head_sha"] = self.workflow_sha
        result = self.select(RUNS=json.dumps(runs))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})
        self.git("checkout", "--detach", self.workflow_sha)
        result = self.select(FAIL_API="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})
        self.git("checkout", "--detach", self.workflow_sha)
        result = self.select(RELEASES='[[{"tag_name":"v1.2.3","draft":false}]]')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.output_values(), {"preserved": "existing-output"})


if __name__ == "__main__":
    unittest.main()
