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

    def test_always_failing_github_lookup_requeues_without_no_pr_diagnosis(self):
        branch = "ciwait-transient-gh-failure-must-not-kill-a-lane"
        gh_calls = []
        sleeps = []

        def run_gh(*args):
            gh_calls.append(args)
            return subprocess.CompletedProcess(
                ["gh", *args],
                1,
                stdout="",
                stderr="token=should-not-be-logged",
            )

        stderr = io.StringIO()
        stdout = io.StringIO()
        with patch.object(self.module, "run_gh", side_effect=run_gh), patch.object(
            self.module.time, "sleep", side_effect=sleeps.append
        ), redirect_stdout(stdout), redirect_stderr(stderr):
            result = self.module.resolve_pr(branch)

        self.assertIsNone(result)
        self.assertEqual(
            len(gh_calls), self.module.PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS
        )
        self.assertEqual(
            sleeps,
            [
                self.module.infrastructure_backoff_seconds(attempt)
                for attempt in range(1, self.module.PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS)
            ],
        )
        payload = json.loads(stdout.getvalue())
        self.assertEqual(payload["status"], "ready")
        self.assertEqual(payload["reason"], "pr-lookup-infrastructure-requeue")
        self.assertEqual(payload["lookup"], "infrastructure-failure")
        self.assertEqual(
            payload["infrastructureAttempts"],
            self.module.PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS,
        )
        diagnostic = stderr.getvalue()
        self.assertIn("infrastructure failure", diagnostic)
        self.assertIn("retrying", diagnostic)
        self.assertIn("hold-and-requeue", diagnostic)
        self.assertNotIn("no PR exists", diagnostic)
        self.assertNotIn("should-not-be-logged", diagnostic)

    def test_timeout_and_unusable_lookup_responses_requeue(self):
        def timeout(*args):
            raise subprocess.TimeoutExpired(
                ["gh", *args], self.module.GH_CALL_TIMEOUT_SECONDS
            )

        def malformed(*args):
            return subprocess.CompletedProcess(
                ["gh", *args],
                0,
                stdout="not-json",
                stderr="",
            )

        for response in (timeout, malformed):
            with self.subTest(response=response.__name__):
                sleeps = []
                stderr = io.StringIO()
                stdout = io.StringIO()
                with (
                    patch.object(self.module, "run_gh", side_effect=response),
                    patch.object(self.module.time, "sleep", side_effect=sleeps.append),
                    redirect_stdout(stdout),
                    redirect_stderr(stderr),
                ):
                    result = self.module.resolve_pr("ciwait-unavailable-github")

                self.assertIsNone(result)
                self.assertEqual(
                    len(sleeps), self.module.PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS - 1
                )
                payload = json.loads(stdout.getvalue())
                self.assertEqual(payload["status"], "ready")
                self.assertEqual(
                    payload["reason"], "pr-lookup-infrastructure-requeue"
                )
                self.assertNotIn("no PR exists", stderr.getvalue())

    def test_infrastructure_retry_can_recover_to_a_found_pr(self):
        responses = iter(
            [
                subprocess.CompletedProcess(
                    ["gh"], 1, stdout="", stderr="temporary outage"
                ),
                subprocess.CompletedProcess(
                    ["gh"], 0, stdout='[{"number": 42, "state": "OPEN"}]', stderr=""
                ),
            ]
        )
        sleeps = []

        with (
            patch.object(
                self.module, "run_gh", side_effect=lambda *args: next(responses)
            ),
            patch.object(self.module.time, "sleep", side_effect=sleeps.append),
        ):
            result = self.module.resolve_pr("ciwait-recovered-github")

        self.assertEqual(result, {"number": 42, "state": "OPEN"})
        self.assertEqual(sleeps, [self.module.infrastructure_backoff_seconds(1)])

    def test_infrastructure_failure_does_not_consume_missing_pr_budget(self):
        responses = iter(
            [
                subprocess.CompletedProcess(
                    ["gh"], 1, stdout="", stderr="temporary outage"
                )
            ]
            + [
                subprocess.CompletedProcess(["gh"], 0, stdout="[]", stderr="")
                for _ in range(self.module.PR_LOOKUP_ATTEMPTS)
            ]
        )
        sleeps = []
        stderr = io.StringIO()

        with (
            patch.object(
                self.module, "run_gh", side_effect=lambda *args: next(responses)
            ),
            patch.object(self.module.time, "sleep", side_effect=sleeps.append),
            redirect_stderr(stderr),
        ):
            with self.assertRaises(SystemExit) as exit_error:
                self.module.resolve_pr("ciwait-recovered-empty")

        self.assertEqual(exit_error.exception.code, 1)
        self.assertEqual(len(sleeps), self.module.PR_LOOKUP_ATTEMPTS)
        self.assertEqual(sleeps[0], self.module.infrastructure_backoff_seconds(1))
        self.assertEqual(
            sleeps[1:],
            [self.module.PR_LOOKUP_INTERVAL_SECONDS]
            * (self.module.PR_LOOKUP_ATTEMPTS - 1),
        )
        self.assertIn("successful PR lookups found no PR", stderr.getvalue())

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
