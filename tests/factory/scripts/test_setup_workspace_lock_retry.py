#!/usr/bin/env python3
"""Tests for setup-workspace retrying transient git lock contention."""

import contextlib
import importlib.util
import io
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


if __name__ == "__main__":
    unittest.main()
