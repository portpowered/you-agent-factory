#!/usr/bin/env python3
"""Behavioral tests for the ci-wait PR lookup gate."""

import importlib.util
import io
import json
import subprocess
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest.mock import patch


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "ci-wait.py"


def load_ci_wait_module():
    spec = importlib.util.spec_from_file_location("ci_wait", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CIWaitPRLookupTest(unittest.TestCase):
    def setUp(self):
        self.module = load_ci_wait_module()

    def test_successful_empty_lookups_exit_1_after_missing_pr_budget(self):
        branch = "ciwait-transient-gh-failure-must-not-kill-a-lane"
        gh_calls = []
        sleeps = []

        def run_gh(*args):
            gh_calls.append(args)
            return subprocess.CompletedProcess(
                ["gh", *args],
                0,
                stdout="[]\n",
                stderr="",
            )

        stderr = io.StringIO()
        stdout = io.StringIO()
        with patch.object(self.module, "run_gh", side_effect=run_gh), patch.object(
            self.module.time, "sleep", side_effect=sleeps.append
        ), redirect_stdout(stdout), redirect_stderr(stderr):
            with self.assertRaises(SystemExit) as exit_error:
                self.module.resolve_pr(branch)

        self.assertEqual(exit_error.exception.code, 1)
        self.assertEqual(len(gh_calls), self.module.PR_LOOKUP_ATTEMPTS)
        self.assertEqual(
            sleeps,
            [self.module.PR_LOOKUP_INTERVAL_SECONDS]
            * (self.module.PR_LOOKUP_ATTEMPTS - 1),
        )
        self.assertEqual(stdout.getvalue(), "")
        diagnostic = stderr.getvalue()
        self.assertIn("successful PR lookups found no PR", diagnostic)
        self.assertIn(branch, diagnostic)
        self.assertNotIn("infrastructure", diagnostic.lower())

    def test_successful_lookup_preserves_preferred_pr_state_selection(self):
        cases = (
            (
                [
                    {"number": 17, "state": "CLOSED"},
                    {"number": 18, "state": "MERGED"},
                    {"number": 19, "state": "OPEN"},
                ],
                {"number": 19, "state": "OPEN"},
            ),
            ([{"number": 20, "state": "MERGED"}], {"number": 20, "state": "MERGED"}),
            ([{"number": 21, "state": "CLOSED"}], {"number": 21, "state": "CLOSED"}),
        )

        for prs, expected in cases:
            with self.subTest(expected=expected):
                def run_gh(*args):
                    return subprocess.CompletedProcess(
                        ["gh", *args],
                        0,
                        stdout=json.dumps(prs),
                        stderr="",
                    )

                with patch.object(self.module, "run_gh", side_effect=run_gh):
                    self.assertEqual(
                        self.module.resolve_pr("ciwait-found-state"),
                        expected,
                    )


if __name__ == "__main__":
    unittest.main()
