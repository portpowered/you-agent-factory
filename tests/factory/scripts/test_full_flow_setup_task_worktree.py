#!/usr/bin/env python3
"""Regression tests for the packaged full-flow worktree setup boundary."""

import json
import queue
import subprocess
import tempfile
import sys
import threading
import time
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

    def run_script(self, task, base="main"):
        return subprocess.run(
            ["python", str(SCRIPT_PATH), task, base],
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

    def test_accepts_slash_in_base_branch_without_changing_task_directory(self):
        git(self.repository, "branch", "bootstrap/full-flow", "main")

        result = self.run_script("task-a", "bootstrap/full-flow")

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["base"], "bootstrap/full-flow")
        self.assertEqual(
            Path(payload["worktree"]),
            self.repository / ".claude" / "worktrees" / "task-a",
        )

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

    def test_hard_killed_lock_owner_returns_bounded_actionable_failure(self):
        first = self.run_script("task-a")
        self.assertEqual(first.returncode, 0, first.stderr)

        lock_path = self.repository / ".git" / "config.lock"
        owner_code = (
            "from pathlib import Path\n"
            "import sys\n"
            "lock = Path(sys.argv[1])\n"
            "with lock.open('w', encoding='utf-8') as handle:\n"
            "    handle.write('owned-by-test\\n')\n"
            "    handle.flush()\n"
            "    print('acquired', flush=True)\n"
            "    sys.stdin.read(1)\n"
        )
        owner = subprocess.Popen(
            [sys.executable, "-c", owner_code, str(lock_path)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        acquired = queue.Queue(maxsize=1)

        def read_acquisition():
            acquired.put(owner.stdout.readline())

        reader = threading.Thread(target=read_acquisition, daemon=True)
        reader.start()
        try:
            try:
                acquisition_line = acquired.get(timeout=30)
            except queue.Empty:
                self.fail("timed out observing hard-kill lock owner acquisition")
            self.assertEqual(acquisition_line.strip(), "acquired")
            started = time.monotonic()
            owner.kill()
            owner.wait(timeout=30)

            result = self.run_script("task-a")
            elapsed = time.monotonic() - started
        finally:
            if owner.poll() is None:
                owner.kill()
                owner.wait(timeout=30)
            for stream in (owner.stdin, owner.stdout, owner.stderr):
                if stream is not None:
                    stream.close()
            reader.join(timeout=5)

        self.assertFalse(reader.is_alive())

        self.assertEqual(result.returncode, 1)
        self.assertLess(elapsed, 5)
        self.assertTrue(lock_path.exists())
        self.assertIn("git config serialization contention", result.stderr)
        self.assertIn(f"resource={lock_path}", result.stderr)
        self.assertIn("owner_liveness=indeterminate", result.stderr)
        self.assertIn("verify no Git or task-worktree setup process", result.stderr)
        self.assertIn(f"remove only {lock_path}", result.stderr)
        self.assertIn("retry", result.stderr)

    def test_live_lock_owner_returns_bounded_contention_without_removing_lock(self):
        first = self.run_script("task-a")
        self.assertEqual(first.returncode, 0, first.stderr)

        lock_path = self.repository / ".git" / "config.lock"
        owner_code = (
            "from pathlib import Path\n"
            "import sys\n"
            "lock = Path(sys.argv[1])\n"
            "with lock.open('w', encoding='utf-8') as handle:\n"
            "    handle.write('owned-by-live-test\\n')\n"
            "    handle.flush()\n"
            "    print('acquired', flush=True)\n"
            "    sys.stdin.read(1)\n"
        )
        owner = subprocess.Popen(
            [sys.executable, "-c", owner_code, str(lock_path)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        acquired = queue.Queue(maxsize=1)

        def read_acquisition():
            acquired.put(owner.stdout.readline())

        reader = threading.Thread(target=read_acquisition, daemon=True)
        reader.start()
        try:
            try:
                acquisition_line = acquired.get(timeout=30)
            except queue.Empty:
                self.fail("timed out observing live lock owner acquisition")
            self.assertEqual(acquisition_line.strip(), "acquired")

            started = time.monotonic()
            result = self.run_script("task-a")
            elapsed = time.monotonic() - started

            self.assertEqual(owner.poll(), None, "live lock owner exited during contention")
            self.assertTrue(lock_path.exists())
            self.assertLess(elapsed, 5)
            self.assertEqual(result.returncode, 1)
            self.assertIn("git config serialization contention", result.stderr)
            self.assertIn(f"resource={lock_path}", result.stderr)
            self.assertIn("owner_liveness=indeterminate", result.stderr)
            self.assertIn("verify no Git or task-worktree setup process", result.stderr)
            self.assertIn(f"remove only {lock_path}", result.stderr)
            self.assertIn("retry", result.stderr)
        finally:
            if owner.poll() is None:
                owner.kill()
                owner.wait(timeout=30)
            for stream in (owner.stdin, owner.stdout, owner.stderr):
                if stream is not None:
                    stream.close()
            reader.join(timeout=5)

        self.assertFalse(reader.is_alive())


if __name__ == "__main__":
    unittest.main()
