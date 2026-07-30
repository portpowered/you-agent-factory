#!/usr/bin/env python3
"""Regression tests for the packaged full-flow worktree setup boundary."""

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = (
    REPO_ROOT
    / "packages"
    / "packaged-factories"
    / "factories"
    / "full-flow"
    / "scripts"
    / "setup-task-worktree.py"
)


def git(repository, *args):
    return subprocess.run(
        ["git", *args],
        cwd=repository,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()


class FullFlowWorktreeSetupTest(unittest.TestCase):
    def setUp(self):
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.repository = Path(self.temporary_directory.name)
        git(self.repository, "init", "-b", "main")
        git(self.repository, "config", "user.email", "full-flow@example.test")
        git(self.repository, "config", "user.name", "Full Flow Test")
        (self.repository / "README.md").write_text("fixture\n", encoding="utf-8")
        git(self.repository, "add", "README.md")
        git(self.repository, "commit", "-m", "fixture")
        # Simulate a packaged invocation whose isolated HOME cannot supply the
        # user's global Git-for-Windows setting.
        git(self.repository, "config", "core.longpaths", "false")

    def tearDown(self):
        self.temporary_directory.cleanup()

    def run_script(self, task):
        return subprocess.run(
            ["python", str(SCRIPT_PATH), task, "main"],
            cwd=self.repository,
            check=False,
            capture_output=True,
            text=True,
        )

    def test_persists_long_path_support_and_creates_matching_worktree(self):
        result = self.run_script("task-a")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        expected = self.repository / ".claude" / "worktrees" / "task-a"
        self.assertEqual(Path(payload["worktree"]), expected)
        self.assertTrue((expected / ".git").exists())
        self.assertEqual(git(self.repository, "config", "--get", "core.longpaths"), "true")

    def test_rejects_branch_names_that_cannot_match_working_directory(self):
        result = self.run_script("feature/task-a")

        self.assertEqual(result.returncode, 1)
        self.assertIn("safe task and base branch names are required", result.stderr)
        self.assertFalse((self.repository / ".claude" / "worktrees").exists())

    def test_concurrent_setups_persist_config_and_create_both_worktrees(self):
        commands = [
            ["python", str(SCRIPT_PATH), task, "main"]
            for task in ("task-a", "task-b")
        ]
        processes = [
            subprocess.Popen(
                command,
                cwd=self.repository,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            for command in commands
        ]

        results = [process.communicate(timeout=30) for process in processes]
        for process, (stdout, stderr) in zip(processes, results):
            self.assertEqual(process.returncode, 0, stderr)
            self.assertEqual(json.loads(stdout)["status"], "ready")
        for task in ("task-a", "task-b"):
            self.assertTrue(
                (self.repository / ".claude" / "worktrees" / task / ".git").exists()
            )
        self.assertEqual(git(self.repository, "config", "--get", "core.longpaths"), "true")


if __name__ == "__main__":
    unittest.main()
