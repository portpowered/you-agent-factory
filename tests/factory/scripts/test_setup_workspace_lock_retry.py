#!/usr/bin/env python3
"""Tests for setup-workspace retrying transient git lock contention."""

import contextlib
import importlib.util
import io
import tempfile
import subprocess
import unittest
from pathlib import Path
from unittest import mock

REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"

INDEX_LOCK_STDERR = (
    "fatal: Unable to create 'C:/repo/.git/index.lock': File exists.\n\n"
    "Another git process seems to be running in this repository."
)


def load_module():
    spec = importlib.util.spec_from_file_location("setup_workspace", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def completed(returncode, stdout="", stderr=""):
    return subprocess.CompletedProcess(["git"], returncode, stdout, stderr)


class GitLockRetryTest(unittest.TestCase):
    def run_git_with(self, results):
        module = load_module()
        queue = list(results)
        calls = []

        def fake_run(*args, **kwargs):
            calls.append(args)
            return queue.pop(0) if len(queue) > 1 else queue[0]

        stderr = io.StringIO()
        with mock.patch.object(module.subprocess, "run", side_effect=fake_run), \
                mock.patch.object(module.time, "sleep") as sleep, \
                contextlib.redirect_stderr(stderr):
            try:
                outcome = module.run_git("merge", "--ff-only", "origin/main")
                error = None
            except RuntimeError as exc:
                outcome, error = None, exc
        return outcome, error, len(calls), sleep, stderr.getvalue()

    def test_transient_lock_contention_then_success(self):
        outcome, error, calls, sleep, log = self.run_git_with([
            completed(128, stderr=INDEX_LOCK_STDERR),
            completed(128, stderr="error: cannot lock ref 'refs/heads/main'"),
            completed(0, stdout="ok"),
        ])
        self.assertIsNone(error)
        self.assertEqual(outcome.stdout, "ok")
        self.assertEqual(calls, 3)
        self.assertEqual(sleep.call_count, 2)
        self.assertIn("attempt 1", log)
        self.assertIn("attempt 2", log)

    def test_non_lock_error_does_not_retry(self):
        _, error, calls, sleep, _ = self.run_git_with([
            completed(1, stderr="fatal: not a git repository"),
        ])
        self.assertIsNotNone(error)
        self.assertEqual(calls, 1)
        sleep.assert_not_called()

    def test_persistent_contention_fails_after_bound(self):
        module = load_module()
        _, error, calls, sleep, _ = self.run_git_with([
            completed(128, stderr=INDEX_LOCK_STDERR),
        ])
        self.assertIsNotNone(error)
        self.assertEqual(calls, module.GIT_LOCK_RETRY_ATTEMPTS)
        self.assertEqual(sleep.call_count, module.GIT_LOCK_RETRY_ATTEMPTS - 1)
        self.assertIn("index.lock", str(error))
        self.assertIn("lock contention persisted", str(error))

    def test_check_false_caller_sees_last_result_without_raising(self):
        module = load_module()
        with mock.patch.object(
            module.subprocess, "run",
            return_value=completed(128, stderr=INDEX_LOCK_STDERR),
        ), mock.patch.object(module.time, "sleep"), \
                contextlib.redirect_stderr(io.StringIO()):
            result = module.run_git("status", check=False)
        self.assertEqual(result.returncode, 128)


def git(args, cwd):
    return subprocess.run(
        ["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
    ).stdout.strip()


class RemoteAdvancesDuringSyncTest(unittest.TestCase):
    def test_sha_missing_locally_is_refetched_before_fast_forward(self):
        module = load_module()
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            remote, local, other = tmp / "remote.git", tmp / "local", tmp / "other"
            git(["init", "--bare", "-b", "main", str(remote)], tmp)
            git(["clone", str(remote), str(local)], tmp)
            for repo in (local,):
                git(["config", "user.email", "t@example.com"], repo)
                git(["config", "user.name", "T"], repo)
            (local / "a.txt").write_text("a\n", encoding="utf-8")
            git(["add", "a.txt"], local)
            git(["commit", "-m", "init"], local)
            git(["push", "-u", "origin", "main"], local)
            git(["clone", str(remote), str(other)], tmp)
            git(["config", "user.email", "t@example.com"], other)
            git(["config", "user.name", "T"], other)

            real_run_git = module.run_git
            advanced = []

            def racing_run_git(*args, **kwargs):
                result = real_run_git(*args, **kwargs)
                if args[:2] == ("fetch", "origin") and not advanced:
                    # A concurrent lane's push lands after our fetch finished.
                    (other / "b.txt").write_text("b\n", encoding="utf-8")
                    git(["add", "b.txt"], other)
                    git(["commit", "-m", "advance"], other)
                    git(["push", "origin", "main"], other)
                    advanced.append(git(["rev-parse", "HEAD"], other))
                return result

            with mock.patch.object(module, "run_git", racing_run_git),                     contextlib.redirect_stderr(io.StringIO()):
                outcome = module.sync_main(local)

            self.assertTrue(advanced)
            self.assertNotIn("not a fast-forward", str(outcome))
            self.assertEqual(outcome.fresh_origin_main_sha, advanced[0])
            self.assertEqual(git(["rev-parse", "main"], local), advanced[0])
            git(["cat-file", "-e", advanced[0] + "^{commit}"], local)


if __name__ == "__main__":
    unittest.main()
