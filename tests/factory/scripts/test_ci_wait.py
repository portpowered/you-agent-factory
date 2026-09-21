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
from contextlib import ExitStack, redirect_stderr, redirect_stdout
from pathlib import Path
from unittest.mock import patch


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "ci-wait.py"
TEST_HEAD = "0123456789abcdef0123456789abcdef01234567"
TEST_HEAD_NEXT = "fedcba9876543210fedcba9876543210fedcba98"
TEST_REPOSITORY = "example/repo"
TEST_REPOSITORY_URL = "https://github.com/example/repo"
TEST_CHECK_LINK = "https://github.com/example/repo/actions/runs/1/job/10"
TEST_EXTRA_LINK = "https://github.com/example/repo/actions/runs/1/job/11"
NO_PROCESS_OUTPUT = object()


def rollup_check(state="SUCCESS", name="Verification", link=TEST_CHECK_LINK):
    """Build the CheckRun shape returned by gh pr view --json statusCheckRollup."""
    terminal = state not in {
        "PENDING",
        "QUEUED",
        "IN_PROGRESS",
        "WAITING",
        "REQUESTED",
        "EXPECTED",
    }
    return {
        "__typename": "CheckRun",
        "name": name,
        "detailsUrl": link,
        "workflowName": "CI",
        "status": "COMPLETED" if terminal else state,
        "conclusion": state if terminal else None,
        "startedAt": "2026-09-05T00:00:00Z",
        "completedAt": "2026-09-05T00:01:00Z" if terminal else None,
    }


def checks_row(state="SUCCESS", name="Verification", link=TEST_CHECK_LINK):
    """Build the row returned by the full gh pr checks response."""
    if state in {
        "PENDING",
        "QUEUED",
        "IN_PROGRESS",
        "WAITING",
        "REQUESTED",
        "EXPECTED",
    }:
        bucket = "pending"
    elif state == "SUCCESS":
        bucket = "pass"
    elif state in {"NEUTRAL", "SKIPPED"}:
        bucket = "skipping"
    else:
        bucket = "fail"
    return {
        "name": name,
        "state": state,
        "bucket": bucket,
        "link": link,
        "workflow": "CI",
        "startedAt": "2026-09-05T00:00:00Z",
        "completedAt": "2026-09-05T00:01:00Z" if bucket != "pending" else None,
    }


def view_payload(head=TEST_HEAD, rollup=None, number=100, state="OPEN"):
    """Build the PR view response consumed by the current-head selector."""
    if rollup is None:
        rollup = [rollup_check()]
    elif not isinstance(rollup, list):
        rollup = [rollup]
    return {
        "number": number,
        "state": state,
        "headRefOid": head,
        "statusCheckRollup": rollup,
    }


def repository_payload(name_with_owner=TEST_REPOSITORY, url=TEST_REPOSITORY_URL):
    return {"nameWithOwner": name_with_owner, "url": url}


def pr_identity_payload(number, state="OPEN", repository_url=TEST_REPOSITORY_URL):
    return {
        "number": number,
        "state": state,
        "url": f"{repository_url}/pull/{number}",
    }


def observation(view, checks, after_view=None):
    """Build the ordered view/checks/view inputs for one snapshot attempt."""
    return (view, checks, after_view if after_view is not None else view)


def actual_fixture(pr_number, observations, direct_view=None, repository=None):
    """Serialize ordered observations for the actual fake-gh boundary."""
    views = []
    checks = []
    if direct_view is not None:
        views.append({"stdout": json.dumps(direct_view)})
    for before, check_response, after in observations:
        views.extend(
            [
                {"stdout": json.dumps(before)},
                {"stdout": json.dumps(after)},
            ]
        )
        if isinstance(check_response, str):
            checks.append({"stdout": check_response})
        elif isinstance(check_response, dict) and "stdout" in check_response:
            checks.append(check_response)
        else:
            checks.append({"stdout": json.dumps(check_response)})
    fixture = {
        "list": [{"stdout": json.dumps([{"number": pr_number, "state": "OPEN"}])}],
        "view": views,
        "checks": checks,
    }
    if repository is not None:
        fixture["repo"] = [{"stdout": json.dumps(repository)}]
    return fixture


def load_ci_wait_module():
    spec = importlib.util.spec_from_file_location("ci_wait", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CIWaitPRLookupTest(unittest.TestCase):
    def setUp(self):
        self.module = load_ci_wait_module()

    def invoke_actual_script_with_fake_gh(
        self,
        fixture,
        clock="stable",
        process_output=NO_PROCESS_OUTPUT,
        lane_name="ciwait-black-box-boundary",
    ):
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
if raw_args[:1] in (["pr"], ["repo"]):
    args = raw_args
elif os.path.basename(sys.argv[0]) == "repo":
    args = ["repo", *raw_args]
else:
    args = ["pr", *raw_args]

ledger_path = os.environ["CI_WAIT_FAKE_GH_LEDGER"]
try:
    with open(ledger_path, encoding="utf-8") as ledger:
        calls = json.load(ledger)
except FileNotFoundError:
    calls = []
calls.append(args)
with open(ledger_path, "w", encoding="utf-8") as ledger:
    json.dump(calls, ledger)

if len(args) < 2 or args[0] not in {"pr", "repo"}:
    print(f"unsupported fake gh command: {args!r}", file=sys.stderr)
    sys.exit(2)
selector = args[:2]
command = args[1] if args[0] == "pr" else "repo"
responses = json.loads(os.environ["CI_WAIT_FAKE_GH_FIXTURE"]).get(command, [])
if not responses:
    print(f"no fake response configured for {args!r}", file=sys.stderr)
    sys.exit(2)
response_index = sum(call[:2] == selector for call in calls) - 1
response = responses[min(response_index, len(responses) - 1)]
if response.get("stdout"):
    print(response["stdout"], end="")
if response.get("stderr"):
    print(response["stderr"], file=sys.stderr)
sys.exit(response.get("returncode", 0))
"""
            fake_gh_path.write_text(fake_gh_source, encoding="utf-8")
            (scratch_path / "sitecustomize.py").write_text(
                f"""import inspect
import time

_calls = 0
_real_monotonic = time.monotonic

def _monotonic():
    global _calls
    if any(frame.filename.endswith("subprocess.py") for frame in inspect.stack(0)):
        return _real_monotonic()
    _calls += 1
    if {clock!r} == "deadline" and _calls > 2:
        return 100000.0
    return 0.0

time.monotonic = _monotonic
time.sleep = lambda _seconds: None
""",
                encoding="utf-8",
            )

            if os.name == "nt":
                # Windows CreateProcess resolves native executables for a
                # shell=False child. The copied Python runtime, named gh.exe,
                # runs the extensionless command fixture as its script.
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
                for command_name in ("pr", "repo"):
                    (scratch_path / command_name).write_text(
                        fake_gh_source,
                        encoding="utf-8",
                    )
            else:
                launcher_path = scratch_path / "gh"
                launcher_path.write_text(
                    f"#!{sys.executable}\n"
                    "import os\n"
                    "fake_gh = os.path.join(os.path.dirname(__file__), 'fake_gh.py')\n"
                    "exec(compile(open(fake_gh, encoding='utf-8').read(), fake_gh, 'exec'))\n",
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
            environment["CI_WAIT_FAKE_GH_FIXTURE"] = json.dumps(fixture)
            environment["PYTHONPATH"] = os.pathsep.join(
                [str(scratch_path), environment.get("PYTHONPATH", "")]
            )
            # On Windows, CreateProcess resolves the executable using the
            # launching process environment rather than the child's replaced
            # environment. The suite is serial and this scope is restored
            # immediately after the one boundary invocation.
            with patch.dict(os.environ, {"PATH": environment["PATH"]}):
                command = [sys.executable, str(SCRIPT_PATH), lane_name]
                if process_output is not NO_PROCESS_OUTPUT:
                    command.append(process_output)
                try:
                    result = subprocess.run(
                        command,
                        cwd=scratch_path,
                        env=environment,
                        capture_output=True,
                        text=True,
                        check=False,
                        timeout=15,
                    )
                except subprocess.TimeoutExpired as error:
                    ledger = (
                        ledger_path.read_text(encoding="utf-8")
                        if ledger_path.exists()
                        else "<missing>"
                    )
                    raise AssertionError(
                        f"actual ci-wait boundary timed out: stdout={error.stdout!r}; "
                        f"stderr={error.stderr!r}; ledger={ledger}"
                    ) from error
            calls = (
                json.loads(ledger_path.read_text(encoding="utf-8"))
                if ledger_path.exists()
                else []
            )
            return result, calls

    def invoke_main(self, branch, run_gh, process_output=NO_PROCESS_OUTPUT):
        """Run the script entrypoint while keeping process outcomes observable."""
        sleeps = []
        stderr = io.StringIO()
        stdout = io.StringIO()
        argv = ["ci-wait.py", branch]
        if process_output is not NO_PROCESS_OUTPUT:
            argv.append(process_output)
        with (
            patch.object(self.module, "run_gh", side_effect=run_gh),
            patch.object(self.module.sys, "argv", argv),
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

    def invoke_main_observations(
        self,
        branch,
        pr_number,
        observations,
        deadline=None,
        no_checks_grace=None,
        process_output=NO_PROCESS_OUTPUT,
    ):
        """Run main with ordered before/checks/after responses and a call ledger."""
        responses = [response for observation in observations for response in observation]
        calls = []
        sleeps = []

        def run_gh(*args):
            calls.append(args)
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout=json.dumps([{"number": pr_number, "state": "OPEN"}]),
                    stderr="",
                )
            response = responses.pop(0)
            if isinstance(response, BaseException):
                raise response
            if isinstance(response, subprocess.CompletedProcess):
                return response
            stdout = response if isinstance(response, str) else json.dumps(response)
            return subprocess.CompletedProcess(
                ["gh", *args], 0, stdout=stdout, stderr=""
            )

        stderr = io.StringIO()
        stdout = io.StringIO()
        argv = ["ci-wait.py", branch]
        if process_output is not NO_PROCESS_OUTPUT:
            argv.append(process_output)
        with ExitStack() as stack:
            stack.enter_context(patch.object(self.module, "run_gh", side_effect=run_gh))
            stack.enter_context(
                patch.object(self.module.sys, "argv", argv)
            )
            stack.enter_context(
                patch.object(self.module.time, "sleep", side_effect=sleeps.append)
            )
            stack.enter_context(patch.object(self.module.time, "monotonic", return_value=0))
            if deadline is not None:
                stack.enter_context(patch.object(self.module, "DEADLINE_SECONDS", deadline))
            if no_checks_grace is not None:
                stack.enter_context(
                    patch.object(self.module, "NO_CHECKS_GRACE_SECONDS", no_checks_grace)
                )
            stack.enter_context(redirect_stdout(stdout))
            stack.enter_context(redirect_stderr(stderr))
            try:
                self.module.main()
            except SystemExit as error:
                exit_code = error.code
            else:
                exit_code = 0
        raw_stdout = stdout.getvalue()
        return (
            exit_code,
            json.loads(raw_stdout) if raw_stdout else None,
            stderr.getvalue(),
            sleeps,
            calls,
        )

    def test_process_output_classifier_accepts_one_identity_and_preserves_absence(
        self,
    ):
        for output in (None, "", "  \n", "worker completed without a PR reference"):
            with self.subTest(output=output):
                intent = self.module.classify_process_output(output)
                self.assertEqual(intent.status, self.module.PRIntentStatus.ABSENT)

        cases = (
            ("PR #42", 42, ""),
            ("pull request: 42", 42, ""),
            ("#42", 42, ""),
            ("42", 42, ""),
            (f"PR URL: {TEST_REPOSITORY_URL}/pull/42", 42, TEST_REPOSITORY),
        )
        for output, expected_number, expected_repository in cases:
            with self.subTest(output=output):
                intent = self.module.classify_process_output(output)
                self.assertEqual(intent.status, self.module.PRIntentStatus.IDENTIFIED)
                self.assertEqual(intent.identity.number, expected_number)
                self.assertEqual(intent.identity.repository, expected_repository)

    def test_process_output_classifier_rejects_ambiguous_malformed_and_oversized(self):
        cases = (
            "PR #42 and PR #43",
            "PR #42 and https://github.com/example/repo/pull/43",
            "PR: unavailable",
            "PR #not-a-number",
            "https://github.com/example/repo/pull/not-a-number",
            "ftp://github.com/example/repo/pull/42",
            "github.com/example/repo/pull/42",
            "PR #42 " + "x" * self.module.MAX_PROCESS_OUTPUT_BYTES,
        )
        for output in cases:
            with self.subTest(output=output[:80]):
                intent = self.module.classify_process_output(output)
                self.assertEqual(intent.status, self.module.PRIntentStatus.INVALID)

    def test_actual_entrypoint_resolves_explicit_local_pr_and_reports_current_head(
        self,
    ):
        pr_number = 241
        view = view_payload(number=pr_number)
        fixture = actual_fixture(
            pr_number,
            [
                observation(view, [checks_row()]),
                observation(view, [checks_row()]),
            ],
            direct_view=pr_identity_payload(pr_number),
            repository=repository_payload(),
        )

        result, calls = self.invoke_actual_script_with_fake_gh(
            fixture,
            process_output=f"Review target: {TEST_REPOSITORY_URL}/pull/{pr_number}",
            lane_name="work-task-name-does-not-match-branch",
        )

        self.assertEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["pr"], pr_number)
        self.assertEqual(payload["prState"], "OPEN")
        self.assertEqual(payload["headRefOid"], TEST_HEAD)
        self.assertEqual(payload["reason"], "checks-terminal")
        self.assertEqual(calls[0], ["repo", "view", "--json", "nameWithOwner,url"])
        self.assertEqual(
            calls[1],
            [
                "pr", "view", str(pr_number), "--repo", TEST_REPOSITORY,
                "--json", "number,state,url",
            ],
        )
        self.assertFalse(any(call[:2] == ["pr", "list"] for call in calls))
        for call in calls[1:]:
            if call[:2] in (["pr", "view"], ["pr", "checks"]):
                self.assertEqual(call[2], str(pr_number))
                self.assertIn("--repo", call)
                self.assertIn(TEST_REPOSITORY, call)

    def test_actual_entrypoint_empty_and_non_pr_output_keep_legacy_branch_lookup(
        self,
    ):
        pr_number = 242
        view = view_payload(number=pr_number)
        for process_output in ("", "completed without a pull request reference"):
            with self.subTest(process_output=process_output):
                fixture = actual_fixture(
                    pr_number,
                    [
                        observation(view, [checks_row()]),
                        observation(view, [checks_row()]),
                    ],
                )
                result, calls = self.invoke_actual_script_with_fake_gh(
                    fixture, process_output=process_output
                )
                self.assertEqual(result.returncode, 0)
                self.assertEqual(json.loads(result.stdout)["pr"], pr_number)
                self.assertEqual(calls[0][:2], ["pr", "list"])
                self.assertFalse(any(call[:2] == ["repo", "view"] for call in calls))

    def test_actual_entrypoint_rejects_unsafe_identity_without_fallback_or_leaks(self):
        cases = (
            "PR #243 and PR #244 token=raw-process-output",
            "PR #243 and https://github.com/example/repo/pull/244",
            "PR #not-a-number token=raw-process-output",
            "ftp://github.com/example/repo/pull/243 token=raw-process-output",
            "PR #243 " + "x" * self.module.MAX_PROCESS_OUTPUT_BYTES,
        )
        for process_output in cases:
            with self.subTest(process_output=process_output[:80]):
                result, calls = self.invoke_actual_script_with_fake_gh(
                    {}, process_output=process_output
                )
                self.assertEqual(calls, [])
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertNotIn("raw-process-output", result.stderr)
                self.assertNotIn("https://github.com", result.stderr)
                self.assertNotIn("ftp://", result.stderr)

    def test_actual_entrypoint_rejects_foreign_pr_url_before_pr_or_branch_query(self):
        process_output = "https://github.com/other/repository/pull/245"
        fixture = {"repo": [{"stdout": json.dumps(repository_payload())}]}

        result, calls = self.invoke_actual_script_with_fake_gh(
            fixture, process_output=process_output
        )

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertEqual(calls, [["repo", "view", "--json", "nameWithOwner,url"]])
        self.assertNotIn(process_output, result.stderr)

    def test_actual_entrypoint_distinguishes_missing_pr_from_infrastructure_requeue(self):
        missing_stderr = (
            "GraphQL: Could not resolve to a PullRequest with the number of 246. "
            "token=must-not-escape"
        )
        missing_fixture = {
            "repo": [{"stdout": json.dumps(repository_payload())}],
            "view": [{"returncode": 1, "stderr": missing_stderr}],
        }
        missing, missing_calls = self.invoke_actual_script_with_fake_gh(
            missing_fixture, process_output="PR #246"
        )

        self.assertEqual(missing.returncode, 1)
        self.assertEqual(missing.stdout, "")
        self.assertIn("was not found", missing.stderr)
        self.assertNotIn("token=must-not-escape", missing.stderr)
        self.assertEqual(
            [call[:2] for call in missing_calls],
            [["repo", "view"], ["pr", "view"]],
        )

        unavailable_fixture = {
            "repo": [{"returncode": 1, "stderr": "token=must-not-escape"}],
        }
        unavailable, unavailable_calls = self.invoke_actual_script_with_fake_gh(
            unavailable_fixture, process_output="#247"
        )

        self.assertEqual(unavailable.returncode, 0)
        payload = json.loads(unavailable.stdout)
        self.assertEqual(
            payload["reason"], "explicit-pr-lookup-infrastructure-requeue"
        )
        self.assertEqual(payload["lookup"], "infrastructure-failure")
        self.assertEqual(payload["infrastructureAttempts"], 3)
        self.assertNotIn("token=must-not-escape", unavailable.stdout)
        self.assertNotIn("token=must-not-escape", unavailable.stderr)
        self.assertEqual(len(unavailable_calls), 3)
        self.assertTrue(all(call[:2] == ["repo", "view"] for call in unavailable_calls))

    def test_actual_entrypoint_requeues_when_exact_pr_view_is_unavailable(self):
        pr_number = 249
        fixture = {
            "repo": [{"stdout": json.dumps(repository_payload())}],
            "view": [{"returncode": 1, "stderr": "token=must-not-escape"}],
        }

        result, calls = self.invoke_actual_script_with_fake_gh(
            fixture, process_output=f"PR #{pr_number}"
        )

        self.assertEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["pr"], pr_number)
        self.assertEqual(payload["reason"], "explicit-pr-lookup-infrastructure-requeue")
        self.assertEqual(payload["lookupStage"], "pull-request")
        self.assertEqual(payload["infrastructureAttempts"], 3)
        self.assertNotIn("token=must-not-escape", result.stdout)
        self.assertNotIn("token=must-not-escape", result.stderr)
        self.assertEqual(
            [call[:2] for call in calls],
            [["repo", "view"], ["pr", "view"], ["pr", "view"], ["pr", "view"]],
        )

    def test_actual_entrypoint_rejects_mismatched_returned_pr_identity(self):
        pr_number = 250
        cases = (
            pr_identity_payload(pr_number + 1),
            pr_identity_payload(
                pr_number,
                repository_url="https://github.com/other/repository",
            ),
        )
        for direct_view in cases:
            with self.subTest(direct_view=direct_view):
                fixture = {
                    "repo": [{"stdout": json.dumps(repository_payload())}],
                    "view": [{"stdout": json.dumps(direct_view)}],
                }
                result, calls = self.invoke_actual_script_with_fake_gh(
                    fixture, process_output=f"PR #{pr_number}"
                )
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertIn("inconsistent identity", result.stderr)
                self.assertNotIn("https://github.com/other", result.stderr)
                self.assertEqual(
                    [call[:2] for call in calls],
                    [["repo", "view"], ["pr", "view"]],
                )

    def test_actual_entrypoint_explicit_merged_pr_keeps_short_circuit(self):
        pr_number = 248
        fixture = {
            "repo": [{"stdout": json.dumps(repository_payload())}],
            "view": [{"stdout": json.dumps(pr_identity_payload(pr_number, "MERGED"))}],
        }

        result, calls = self.invoke_actual_script_with_fake_gh(
            fixture, process_output=str(pr_number)
        )

        self.assertEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["pr"], pr_number)
        self.assertEqual(payload["prState"], "MERGED")
        self.assertEqual(payload["reason"], "pr-merged")
        self.assertEqual(
            [call[:2] for call in calls],
            [["repo", "view"], ["pr", "view"]],
        )

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
        view = view_payload()
        check = checks_row()

        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 100, "state": "OPEN"}]',
                    stderr="",
                )
            if args[1] == "view":
                return subprocess.CompletedProcess(
                    ["gh", *args], 0, stdout=json.dumps(view), stderr=""
                )
            return subprocess.CompletedProcess(
                ["gh", *args], 0, stdout=json.dumps([check]), stderr=""
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-terminal", run_gh
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "checks-terminal")
        self.assertEqual(json.loads(stdout)["prState"], "OPEN")
        self.assertEqual(json.loads(stdout)["checks"], 1)
        self.assertEqual(sleeps, [self.module.POLL_INTERVAL_SECONDS])
        self.assertIn("terminal", stderr)

    def test_f06_f07_actual_entrypoint_preserves_terminal_green_and_red_boundary(self):
        for state in ("SUCCESS", "FAILURE"):
            rollup = rollup_check(state=state)
            check = checks_row(state=state)
            fixture = {
                "list": [{"stdout": json.dumps([{"number": 100, "state": "OPEN"}])}],
                "view": [
                    {"stdout": json.dumps(view_payload(rollup=rollup))}
                    for _ in range(4)
                ],
                "checks": [
                    {"stdout": json.dumps([check])}
                    for _ in range(2)
                ],
            }
            with self.subTest(state=state):
                result, calls = self.invoke_actual_script_with_fake_gh(fixture)

                self.assertEqual(result.returncode, 0)
                payload = json.loads(result.stdout)
                self.assertEqual(payload["reason"], "checks-terminal")
                self.assertEqual(payload["pr"], 100)
                self.assertEqual(payload["prState"], "OPEN")
                self.assertEqual(payload["checks"], 1)
                self.assertEqual(payload["headRefOid"], TEST_HEAD)
                self.assertEqual(payload["checkIdentities"][0]["state"], state)
                self.assertIsNone(payload["uncertainty"])
                self.assertNotIn("passed", result.stdout)
                self.assertNotIn("mergeAuthorized", result.stdout)
                self.assertEqual(
                    [call[:2] for call in calls],
                    [
                        ["pr", "list"],
                        ["pr", "view"],
                        ["pr", "checks"],
                        ["pr", "view"],
                        ["pr", "view"],
                        ["pr", "checks"],
                        ["pr", "view"],
                    ],
                )
                self.assertEqual(len(calls), 7)
                self.assertEqual(result.stderr.count("terminal"), 2)

    def test_actual_entrypoint_current_head_matrix_stays_within_fixture_budget(self):
        pending_extra = checks_row(
            state="IN_PROGRESS", name="Documentation", link=TEST_EXTRA_LINK
        )
        first_view = view_payload(number=121, rollup=[rollup_check()])
        delayed_view = view_payload(
            number=121,
            rollup=[
                rollup_check(),
                rollup_check(
                    state="IN_PROGRESS", name="Documentation", link=TEST_EXTRA_LINK
                ),
            ],
        )
        complete_view = view_payload(
            number=121,
            rollup=[
                rollup_check(),
                rollup_check(
                    state="SUCCESS", name="Documentation", link=TEST_EXTRA_LINK
                ),
            ],
        )
        old_view = view_payload(number=122, head=TEST_HEAD, rollup=[rollup_check()])
        new_view = view_payload(
            number=122,
            head=TEST_HEAD_NEXT,
            rollup=[rollup_check(link=TEST_EXTRA_LINK)],
        )
        rerun_success_view = view_payload(
            number=123, rollup=[rollup_check(state="SUCCESS")]
        )
        rerun_failure_view = view_payload(
            number=123, rollup=[rollup_check(state="FAILURE")]
        )
        unknown_state = checks_row(name="Future Check", link=TEST_EXTRA_LINK)
        unknown_state["state"] = "MYSTERY"
        unknown_state["bucket"] = "pass"
        unavailable = {
            "returncode": 1,
            "stdout": "",
            "stderr": "token=must-not-escape",
        }
        mismatch_view = view_payload(
            number=124, rollup=[rollup_check(state="SUCCESS")]
        )

        scenarios = (
            (
                "F01 pending extra",
                120,
                [
                    observation(
                        view_payload(number=120),
                        [checks_row(), pending_extra],
                    )
                ],
                "deadline",
                "deadline-requeue",
            ),
            (
                "F02 delayed registration",
                121,
                [
                    observation(first_view, [checks_row()]),
                    observation(
                        delayed_view,
                        [checks_row(), pending_extra],
                    ),
                    observation(
                        complete_view,
                        [
                            checks_row(),
                            checks_row(
                                state="SUCCESS",
                                name="Documentation",
                                link=TEST_EXTRA_LINK,
                            ),
                        ],
                    ),
                    observation(
                        complete_view,
                        [
                            checks_row(),
                            checks_row(
                                state="SUCCESS",
                                name="Documentation",
                                link=TEST_EXTRA_LINK,
                            ),
                        ],
                    ),
                ],
                "stable",
                "checks-terminal",
            ),
            (
                "F03 head read race",
                122,
                [observation(old_view, [checks_row()], new_view)],
                "deadline",
                "deadline-requeue",
            ),
            (
                "F04 new head",
                122,
                [
                    observation(old_view, [checks_row()]),
                    observation(new_view, [checks_row(link=TEST_EXTRA_LINK)]),
                    observation(new_view, [checks_row(link=TEST_EXTRA_LINK)]),
                ],
                "stable",
                "checks-terminal",
            ),
            (
                "F05 rerun",
                123,
                [
                    observation(rerun_success_view, [checks_row(state="SUCCESS")]),
                    observation(rerun_failure_view, [checks_row(state="FAILURE")]),
                    observation(rerun_failure_view, [checks_row(state="FAILURE")]),
                ],
                "stable",
                "checks-terminal",
            ),
            (
                "F08 unknown state",
                125,
                [observation(view_payload(number=125), [checks_row(), unknown_state])],
                "deadline",
                "deadline-requeue",
            ),
            (
                "F09 malformed checks",
                126,
                [observation(view_payload(number=126), "not-json")],
                "deadline",
                "deadline-requeue",
            ),
            (
                "F10 empty checks",
                127,
                [observation(view_payload(number=127, rollup=[]), [])],
                "deadline",
                "no-checks-reported",
            ),
            (
                "F11 unavailable checks",
                128,
                [observation(view_payload(number=128), unavailable)],
                "deadline",
                "deadline-requeue",
            ),
            (
                "F12 source mismatch",
                124,
                [observation(mismatch_view, [checks_row(state="FAILURE")])],
                "deadline",
                "deadline-requeue",
            ),
        )

        total_calls = 0
        for name, pr_number, observations, clock, expected_reason in scenarios:
            with self.subTest(case=name):
                result, calls = self.invoke_actual_script_with_fake_gh(
                    actual_fixture(pr_number, observations), clock=clock
                )

                self.assertEqual(result.returncode, 0)
                payload = json.loads(result.stdout)
                self.assertEqual(payload["reason"], expected_reason)
                self.assertEqual(payload["pr"], pr_number)
                self.assertNotIn("passed", result.stdout)
                self.assertNotIn("mergeAuthorized", result.stdout)
                for call in calls:
                    if call[1] == "view":
                        self.assertEqual(
                            call[3:],
                            ["--json", self.module.PR_VIEW_JSON_FIELDS],
                        )
                    elif call[1] == "checks":
                        self.assertEqual(
                            call[3:],
                            ["--json", self.module.PR_CHECKS_JSON_FIELDS],
                        )
                if name == "F01 pending extra":
                    self.assertEqual(payload["headRefOid"], TEST_HEAD)
                    self.assertEqual(payload["pendingChecks"][0]["name"], "Documentation")
                    self.assertIn(
                        f"CheckRun|Documentation|{TEST_EXTRA_LINK}",
                        result.stderr,
                    )
                elif name == "F02 delayed registration":
                    self.assertEqual(payload["checks"], 2)
                    self.assertIn("non-terminal", result.stderr)
                elif name == "F03 head read race":
                    self.assertEqual(
                        payload["uncertainty"]["reason"],
                        "head-changed-during-observation",
                    )
                elif name == "F04 new head":
                    self.assertEqual(payload["headRefOid"], TEST_HEAD_NEXT)
                    self.assertEqual(
                        payload["checkIdentities"][0]["link"], TEST_EXTRA_LINK
                    )
                elif name == "F05 rerun":
                    self.assertEqual(
                        payload["checkIdentities"][0]["state"], "FAILURE"
                    )
                elif name == "F08 unknown state":
                    self.assertEqual(
                        payload["uncertainty"]["reason"],
                        "checks-unknown-check-state",
                    )
                elif name == "F09 malformed checks":
                    self.assertEqual(payload["uncertainty"]["reason"], "checks-malformed")
                elif name == "F10 empty checks":
                    self.assertIsNotNone(payload["uncertainty"])
                elif name == "F11 unavailable checks":
                    self.assertEqual(payload["uncertainty"]["reason"], "checks-unavailable")
                    self.assertNotIn("token=must-not-escape", result.stderr)
                elif name == "F12 source mismatch":
                    self.assertEqual(
                        payload["uncertainty"]["reason"], "check-state-mismatch"
                    )
                total_calls += len(calls)

        self.assertLessEqual(total_calls, 100)

    def test_f01_pending_extra_check_blocks_terminal_and_names_head_and_identity(self):
        pending_extra = checks_row(
            state="IN_PROGRESS", name="Documentation", link=TEST_EXTRA_LINK
        )

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f01-pending-extra",
            110,
            [
                observation(
                    view_payload(number=110, rollup=[rollup_check()]),
                    [checks_row(), pending_extra],
                    view_payload(number=110, rollup=[rollup_check()]),
                )
            ],
            deadline=0,
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertNotEqual(payload["reason"], "checks-terminal")
        self.assertEqual(payload["headRefOid"], TEST_HEAD)
        self.assertEqual(payload["checks"], 2)
        self.assertEqual(payload["pendingChecks"][0]["name"], "Documentation")
        self.assertEqual(payload["pendingChecks"][0]["state"], "IN_PROGRESS")
        self.assertEqual(
            payload["pendingChecks"][0]["identity"],
            f"CheckRun|Documentation|{TEST_EXTRA_LINK}",
        )
        self.assertIn(TEST_HEAD, stderr)
        self.assertIn(f"CheckRun|Documentation|{TEST_EXTRA_LINK}", stderr)
        self.assertEqual(sleeps, [])
        self.assertEqual(len(calls), 4)
        self.assertNotIn("--required", [argument for call in calls for argument in call])

    def test_f02_delayed_registration_invalidates_partial_rollup_candidate(self):
        first_view = view_payload(number=111, rollup=[rollup_check()])
        delayed_view = view_payload(
            number=111,
            rollup=[
                rollup_check(),
                rollup_check(
                    state="IN_PROGRESS", name="Documentation", link=TEST_EXTRA_LINK
                ),
            ],
        )
        first_checks = [checks_row()]
        delayed_checks = [
            checks_row(),
            checks_row(state="IN_PROGRESS", name="Documentation", link=TEST_EXTRA_LINK),
        ]
        complete_checks = [
            checks_row(),
            checks_row(
                state="SUCCESS", name="Documentation", link=TEST_EXTRA_LINK
            ),
        ]
        complete_view = view_payload(
            number=111,
            rollup=[
                rollup_check(),
                rollup_check(state="SUCCESS", name="Documentation", link=TEST_EXTRA_LINK),
            ],
        )

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f02-delayed-registration",
            111,
            [
                observation(first_view, first_checks),
                observation(delayed_view, delayed_checks),
                observation(complete_view, complete_checks),
                observation(complete_view, complete_checks),
            ],
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "checks-terminal")
        self.assertEqual(payload["headRefOid"], TEST_HEAD)
        self.assertEqual(payload["checks"], 2)
        self.assertEqual(sleeps, [self.module.POLL_INTERVAL_SECONDS] * 3)
        self.assertEqual(len(calls), 13)
        self.assertIn("non-terminal", stderr)
        self.assertIn("same-head convergence", stderr)

    def test_f03_head_change_during_selection_discards_stale_observation(self):
        before = view_payload(number=112, head=TEST_HEAD, rollup=[rollup_check()])
        after = view_payload(number=112, head=TEST_HEAD_NEXT, rollup=[rollup_check()])

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f03-head-race",
            112,
            [observation(before, [checks_row()], after)],
            deadline=0,
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertEqual(payload["headRefOid"], TEST_HEAD_NEXT)
        self.assertEqual(
            payload["uncertainty"]["reason"], "head-changed-during-observation"
        )
        self.assertEqual(
            payload["uncertainty"]["observedHeads"], [TEST_HEAD, TEST_HEAD_NEXT]
        )
        self.assertIn(TEST_HEAD, stderr)
        self.assertIn(TEST_HEAD_NEXT, stderr)
        self.assertEqual(sleeps, [])
        self.assertEqual(len(calls), 4)

    def test_f04_new_head_after_terminal_candidate_requires_new_head_convergence(self):
        old_view = view_payload(number=113, head=TEST_HEAD, rollup=[rollup_check()])
        new_view = view_payload(
            number=113,
            head=TEST_HEAD_NEXT,
            rollup=[rollup_check(link=TEST_EXTRA_LINK)],
        )
        old_checks = [checks_row()]
        new_checks = [checks_row(link=TEST_EXTRA_LINK)]

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f04-new-head",
            113,
            [
                observation(old_view, old_checks),
                observation(new_view, new_checks),
                observation(new_view, new_checks),
            ],
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "checks-terminal")
        self.assertEqual(payload["headRefOid"], TEST_HEAD_NEXT)
        self.assertEqual(payload["checkIdentities"][0]["link"], TEST_EXTRA_LINK)
        self.assertEqual(sleeps, [self.module.POLL_INTERVAL_SECONDS] * 2)
        self.assertEqual(len(calls), 10)
        self.assertIn("same-head convergence", stderr)

    def test_f05_rerun_state_change_cannot_reuse_terminal_candidate(self):
        first_view = view_payload(number=114, rollup=[rollup_check(state="SUCCESS")])
        rerun_view = view_payload(number=114, rollup=[rollup_check(state="FAILURE")])

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f05-rerun",
            114,
            [
                observation(first_view, [checks_row(state="SUCCESS")]),
                observation(rerun_view, [checks_row(state="FAILURE")]),
                observation(rerun_view, [checks_row(state="FAILURE")]),
            ],
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "checks-terminal")
        self.assertEqual(payload["headRefOid"], TEST_HEAD)
        self.assertEqual(payload["checkIdentities"][0]["state"], "FAILURE")
        self.assertEqual(payload["checkIdentities"][0]["bucket"], "fail")
        self.assertEqual(sleeps, [self.module.POLL_INTERVAL_SECONDS] * 2)
        self.assertEqual(len(calls), 10)
        self.assertNotIn("passed", json.dumps(payload))
        self.assertNotIn("mergeAuthorized", json.dumps(payload))

    def test_f08_unknown_identity_or_state_is_explicit_uncertainty(self):
        view = view_payload(number=115, rollup=[rollup_check()])
        cases = []
        unknown_state = checks_row(name="Future Check", link=TEST_EXTRA_LINK)
        unknown_state["state"] = "MYSTERY"
        unknown_state["bucket"] = "pass"
        cases.append((unknown_state, "checks-unknown-check-state"))
        unknown_identity = checks_row(name="Unidentified Check", link=None)
        unknown_identity["workflow"] = None
        cases.append((unknown_identity, "checks-unknown-check-identity"))

        for unknown_check, expected_reason in cases:
            with self.subTest(reason=expected_reason):
                exit_code, payload, stderr, sleeps, calls = (
                    self.invoke_main_observations(
                        "ciwait-f08-unknown",
                        115,
                        [observation(view, [checks_row(), unknown_check])],
                        deadline=0,
                    )
                )

                self.assertEqual(exit_code, 0)
                self.assertEqual(payload["reason"], "deadline-requeue")
                self.assertEqual(payload["uncertainty"]["reason"], expected_reason)
                self.assertNotEqual(payload["reason"], "checks-terminal")
                self.assertEqual(sleeps, [])
                self.assertEqual(len(calls), 4)

    def test_f09_malformed_check_read_is_explicit_uncertainty(self):
        view = view_payload(number=116, rollup=[rollup_check()])

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f09-malformed",
            116,
            [observation(view, "not-json")],
            deadline=0,
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertEqual(payload["uncertainty"]["reason"], "checks-malformed")
        self.assertNotEqual(payload["reason"], "checks-terminal")
        self.assertIn("unparseable JSON", stderr)
        self.assertEqual(sleeps, [])
        self.assertEqual(len(calls), 4)

    def test_f12_source_state_mismatch_never_releases_terminal(self):
        view = view_payload(number=117, rollup=[rollup_check(state="SUCCESS")])

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f12-source-mismatch",
            117,
            [observation(view, [checks_row(state="FAILURE")])],
            deadline=0,
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertEqual(payload["uncertainty"]["reason"], "check-state-mismatch")
        self.assertNotEqual(payload["reason"], "checks-terminal")
        self.assertEqual(sleeps, [])
        self.assertEqual(len(calls), 4)

    def test_f11_unavailable_check_read_is_bounded_and_redacts_dependency_output(self):
        view = view_payload(number=118, rollup=[rollup_check()])
        unavailable = subprocess.CompletedProcess(
            ["gh"], 1, stdout="", stderr="token=must-not-escape"
        )

        exit_code, payload, stderr, sleeps, calls = self.invoke_main_observations(
            "ciwait-f11-unavailable",
            118,
            [observation(view, unavailable)],
            deadline=0,
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertEqual(payload["uncertainty"]["reason"], "checks-unavailable")
        self.assertNotIn("token=must-not-escape", stderr)
        self.assertEqual(sleeps, [])
        self.assertEqual(len(calls), 4)

    def test_f13_actual_entrypoint_preserves_no_pr_terminal_failure(self):
        fixture = {
            "list": [{"stdout": "[]"}],
            "view": [],
            "checks": [],
        }

        result, calls = self.invoke_actual_script_with_fake_gh(fixture)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("successful PR lookups found no PR", result.stderr)
        self.assertEqual(len(calls), self.module.PR_LOOKUP_ATTEMPTS)

    def test_f14_actual_entrypoint_preserves_infrastructure_requeue(self):
        fixture = {
            "list": [
                {
                    "returncode": 1,
                    "stdout": "",
                    "stderr": "token=must-not-escape",
                }
            ],
            "view": [],
            "checks": [],
        }

        result, calls = self.invoke_actual_script_with_fake_gh(fixture)

        self.assertEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["reason"], "pr-lookup-infrastructure-requeue")
        self.assertEqual(payload["infrastructureAttempts"], 3)
        self.assertNotIn("token=must-not-escape", result.stdout)
        self.assertNotIn("token=must-not-escape", result.stderr)
        self.assertEqual(len(calls), 3)

    def test_f15_actual_deadline_requeues_pending_current_head_evidence(self):
        pending_view = view_payload(
            number=119, rollup=[rollup_check(state="IN_PROGRESS")]
        )
        fixture = {
            "list": [{"stdout": json.dumps([{"number": 119, "state": "OPEN"}])}],
            "view": [{"stdout": json.dumps(pending_view)}],
            "checks": [
                {"stdout": json.dumps([checks_row(state="IN_PROGRESS")])}
            ],
        }

        result, calls = self.invoke_actual_script_with_fake_gh(
            fixture, clock="deadline"
        )

        self.assertEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["reason"], "deadline-requeue")
        self.assertEqual(payload["headRefOid"], TEST_HEAD)
        self.assertEqual(payload["pendingChecks"][0]["state"], "IN_PROGRESS")
        self.assertNotIn("checks-terminal", result.stdout)
        self.assertEqual(len(calls), 4)

    def test_main_pending_checks_wait_then_keep_terminal_policy(self):
        pending_view = view_payload(
            rollup=rollup_check(state="IN_PROGRESS"), number=101
        )
        terminal_view = view_payload(number=101)
        pending_checks = [checks_row(state="IN_PROGRESS")]
        terminal_checks = [checks_row()]
        observations = iter(
            [
                json.dumps(pending_view),
                json.dumps(pending_checks),
                json.dumps(pending_view),
                json.dumps(terminal_view),
                json.dumps(terminal_checks),
                json.dumps(terminal_view),
                json.dumps(terminal_view),
                json.dumps(terminal_checks),
                json.dumps(terminal_view),
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
                ["gh", *args], 0, stdout=next(observations), stderr=""
            )

        exit_code, stdout, stderr, sleeps = self.invoke_main(
            "ciwait-routing-pending", run_gh
        )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "checks-terminal")
        self.assertEqual(
            sleeps,
            [self.module.POLL_INTERVAL_SECONDS] * 2,
        )
        self.assertIn("non-terminal", stderr)

    def test_main_deadline_releases_pending_checks_for_requeue(self):
        pending_view = view_payload(
            rollup=rollup_check(state="IN_PROGRESS"), number=102
        )
        observations = iter(
            [
                json.dumps(pending_view),
                json.dumps([checks_row(state="IN_PROGRESS")]),
                json.dumps(pending_view),
            ]
        )

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
                0, stdout=next(observations), stderr=""
            )

        with patch.object(self.module, "DEADLINE_SECONDS", 0):
            exit_code, stdout, stderr, sleeps = self.invoke_main(
                "ciwait-routing-deadline", run_gh
            )

        self.assertEqual(exit_code, 0)
        self.assertEqual(json.loads(stdout)["reason"], "deadline-requeue")
        self.assertEqual(sleeps, [])
        self.assertIn("hold-and-requeue", stderr)

    def test_f10_empty_check_set_releases_without_failure(self):
        empty_view = view_payload(rollup=[], number=103)
        observations = iter(
            [
                json.dumps(empty_view),
                "[]",
                json.dumps(empty_view),
            ]
        )

        def run_gh(*args):
            if args[1] == "list":
                return subprocess.CompletedProcess(
                    ["gh", *args],
                    0,
                    stdout='[{"number": 103, "state": "OPEN"}]',
                    stderr="",
                )
            return subprocess.CompletedProcess(
                ["gh", *args], 0, stdout=next(observations), stderr=""
            )

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
