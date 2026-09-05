#!/usr/bin/env python3
"""Behavioral tests for the ci-wait PR lookup gate."""

import importlib.util
import io
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
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

    def invoke_actual_script_with_fake_gh(self, check):
        """Run the real script entrypoint against a scratch-owned fake gh."""
        with tempfile.TemporaryDirectory(prefix="ci-wait-fake-gh-") as scratch:
            scratch_path = Path(scratch)
            ledger_path = scratch_path / "gh-calls.json"
            fake_gh_path = scratch_path / "fake_gh.py"
            fake_gh_source = """#!/usr/bin/env python3
import json
import os
import sys

raw_args = sys.argv[1:]
args = raw_args if raw_args[:1] == ["pr"] else ["pr", *raw_args]

ledger_path = os.environ["CI_WAIT_FAKE_GH_LEDGER"]
try:
    with open(ledger_path, encoding="utf-8") as ledger:
        calls = json.load(ledger)
except FileNotFoundError:
    calls = []
calls.append(args)
with open(ledger_path, "w", encoding="utf-8") as ledger:
    json.dump(calls, ledger)

if args[:2] == ["pr", "list"]:
    print(json.dumps([{"number": 100, "state": "OPEN"}]))
elif args[:2] == ["pr", "checks"]:
    print(os.environ["CI_WAIT_FAKE_GH_CHECKS"])
else:
    print(f"unsupported fake gh command: {args!r}", file=sys.stderr)
    sys.exit(2)
"""
            fake_gh_path.write_text(fake_gh_source, encoding="utf-8")

            if os.name == "nt":
                # Windows CreateProcess resolves native executables for a
                # shell=False child. A copied Python runtime, named gh.exe,
                # runs the extensionless `pr` fixture as its script.
                launcher_path = scratch_path / "gh.exe"
                shutil.copy2(sys.executable, launcher_path)
                for runtime_name in (
                    f"python{sys.version_info.major}{sys.version_info.minor}.dll",
                    "vcruntime140.dll",
                    "vcruntime140_1.dll",
                ):
                    runtime_path = Path(sys.executable).with_name(runtime_name)
                    if runtime_path.exists():
                        shutil.copy2(runtime_path, scratch_path / runtime_name)
                (scratch_path / "pr").write_text(
                    fake_gh_source,
                    encoding="utf-8",
                )
            else:
                launcher_path = scratch_path / "gh"
                launcher_path.write_text(
                    f"#!{sys.executable}\n"
                    "exec(open(__file__.replace('gh', 'fake_gh.py')).read())\n",
                    encoding="utf-8",
                )
                launcher_path.chmod(
                    launcher_path.stat().st_mode
                    | stat.S_IXUSR
                    | stat.S_IXGRP
                    | stat.S_IXOTH
                )

            environment = os.environ.copy()
            environment["PATH"] = os.pathsep.join(
                [str(scratch_path), environment.get("PATH", "")]
            )
            environment["CI_WAIT_FAKE_GH_LEDGER"] = str(ledger_path)
            environment["CI_WAIT_FAKE_GH_CHECKS"] = json.dumps([check])
            # On Windows, CreateProcess resolves the executable using the
            # launching process environment rather than the child's replaced
            # environment. The suite is serial and this scope is restored
            # immediately after the one boundary invocation.
            with patch.dict(os.environ, {"PATH": environment["PATH"]}):
                result = subprocess.run(
                    [sys.executable, str(SCRIPT_PATH), "ciwait-black-box-boundary"],
                    cwd=scratch_path,
                    env=environment,
                    capture_output=True,
                    text=True,
                    check=False,
                    timeout=15,
                )
            calls = json.loads(ledger_path.read_text(encoding="utf-8"))
            return result, calls

    def invoke_main(self, branch, run_gh):
        """Run the script entrypoint while keeping process outcomes observable."""
        sleeps = []
        stderr = io.StringIO()
        stdout = io.StringIO()
        with (
            patch.object(self.module, "run_gh", side_effect=run_gh),
            patch.object(self.module.sys, "argv", ["ci-wait.py", branch]),
            patch.object(self.module.time, "sleep", side_effect=sleeps.append),
            patch.object(self.module.time, "monotonic", return_value=0),
            redirect_stdout(stdout),
            redirect_stderr(stderr),
        ):
            try:
                self.module.main()
            except SystemExit as error:
                exit_code = error.code
            else:
                exit_code = 0
        return exit_code, stdout.getvalue(), stderr.getvalue(), sleeps

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

    def test_main_infrastructure_exhaustion_is_successful_requeue(self):
        def run_gh(*args):
            return subprocess.CompletedProcess(
                ["gh", *args],
                1,
                stdout="",
                stderr="temporary outage",
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-infrastructure", run_gh
        )

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout)
        self.assertEqual(payload["reason"], "pr-lookup-infrastructure-requeue")
        self.assertEqual(payload["status"], "ready")
        self.assertEqual(
            sleeps,
            [
                self.module.infrastructure_backoff_seconds(attempt)
                for attempt in range(1, self.module.PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS)
            ],
        )
        self.assertIn("hold-and-requeue", stderr)
        self.assertNotIn("successful PR lookups found no PR", stderr)

    def test_main_successful_missing_pr_remains_terminal_failure(self):
        def run_gh(*args):
            return subprocess.CompletedProcess(
                ["gh", *args],
                0,
                stdout="[]",
                stderr="",
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-missing", run_gh
        )

        self.assertEqual(exit_code, 1)
        self.assertEqual(stdout, "")
        self.assertEqual(
            sleeps,
            [self.module.PR_LOOKUP_INTERVAL_SECONDS]
            * (self.module.PR_LOOKUP_ATTEMPTS - 1),
        )
        self.assertIn("successful PR lookups found no PR", stderr)
        self.assertNotIn("hold-and-requeue", stderr)

    def test_main_merged_and_closed_prs_keep_short_circuit_results(self):
        for state, reason in (("MERGED", "pr-merged"), ("CLOSED", "pr-closed")):
            with self.subTest(state=state):
                def run_gh(*args):
                    return subprocess.CompletedProcess(
                        ["gh", *args],
                        0,
                        stdout=json.dumps([{"number": 99, "state": state}]),
                        stderr="",
                    )

                exit_code, stdout, stderr, sleeps = self.invoke_main(
                    "ciwait-routing-closed-state", run_gh
                )

                self.assertEqual(exit_code, 0)
                self.assertEqual(
                    json.loads(stdout),
                    {
                        "status": "ready",
                        "pr": 99,
                        "prState": state,
                        "reason": reason,
                    },
                )
                self.assertEqual(sleeps, [])
                self.assertNotIn("gh pr checks", stderr)

    def test_main_open_pr_releases_after_terminal_checks(self):
        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 100, "state": "OPEN"}]',
                    stderr="",
                )
            return subprocess.CompletedProcess(
                ["gh", *args],
                0,
                stdout='[{"name": "Verification", "state": "SUCCESS", "bucket": "pass"}]',
                stderr="",
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-terminal", run_gh
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "checks-terminal")
        self.assertEqual(json.loads(stdout)["prState"], "OPEN")
        self.assertEqual(json.loads(stdout)["checks"], 1)
        self.assertEqual(sleeps, [])
        self.assertIn("terminal", stderr)

    def test_actual_entrypoint_preserves_terminal_green_and_red_boundary(self):
        for check in (
            {"name": "Verification", "state": "SUCCESS", "bucket": "pass"},
            {"name": "Verification", "state": "FAILURE", "bucket": "fail"},
        ):
            with self.subTest(state=check["state"]):
                result, calls = self.invoke_actual_script_with_fake_gh(check)

                self.assertEqual(result.returncode, 0)
                payload = json.loads(result.stdout)
                self.assertEqual(
                    payload,
                    {
                        "status": "ready",
                        "pr": 100,
                        "prState": "OPEN",
                        "reason": "checks-terminal",
                        "checks": 1,
                    },
                )
                self.assertNotIn("passed", result.stdout)
                self.assertNotIn("mergeAuthorized", result.stdout)
                self.assertEqual(calls[0][:2], ["pr", "list"])
                self.assertEqual(calls[1][:2], ["pr", "checks"])
                self.assertEqual(len(calls), 2)
                self.assertEqual(result.stderr.count("terminal"), 1)

    def test_main_pending_checks_wait_then_keep_terminal_policy(self):
        checks = iter(
            [
                '[{"name": "Verification", "state": "IN_PROGRESS", "bucket": "pending"}]',
                '[{"name": "Verification", "state": "SUCCESS", "bucket": "pass"}]',
            ]
        )

        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 101, "state": "OPEN"}]',
                    stderr="",
                )
            return subprocess.CompletedProcess(
                ["gh", *args], 0, stdout=next(checks), stderr=""
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-pending", run_gh
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "checks-terminal")
        self.assertEqual(sleeps, [self.module.POLL_INTERVAL_SECONDS])
        self.assertIn("non-terminal", stderr)

    def test_main_deadline_releases_pending_checks_for_requeue(self):
        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 102, "state": "OPEN"}]',
                    stderr="",
                )
            return subprocess.CompletedProcess(
                ["gh", *args],
                0,
                stdout='[{"name": "Verification", "state": "IN_PROGRESS", "bucket": "pending"}]',
                stderr="",
            )

        with patch.object(self.module, "DEADLINE_SECONDS", 0):
            exit_code, stdout, stderr, sleeps = self.invoke_main(
                "ciwait-routing-deadline", run_gh
            )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "deadline-requeue")
        self.assertEqual(sleeps, [])
        self.assertIn("hold-and-requeue", stderr)

    def test_main_no_checks_after_grace_releases_without_failure(self):
        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 103, "state": "OPEN"}]',
                    stderr="",
                )
            return subprocess.CompletedProcess(["gh", *args], 0, stdout="[]", stderr="")

        with patch.object(self.module, "NO_CHECKS_GRACE_SECONDS", 0):
            exit_code, stdout, stderr, sleeps = self.invoke_main(
                "ciwait-routing-no-checks", run_gh
            )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "no-checks-reported")
        self.assertEqual(sleeps, [])
        self.assertIn("no checks", stderr)


if __name__ == "__main__":
    unittest.main()
