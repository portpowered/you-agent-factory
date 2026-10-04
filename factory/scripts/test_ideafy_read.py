"""Component-isolated retry behavior; all effects are invocation-local doubles."""

import importlib.util
import io
from pathlib import Path
import subprocess
import unittest


spec = importlib.util.spec_from_file_location("ideafy_read", Path(__file__).with_name("ideafy-read.py"))
read = importlib.util.module_from_spec(spec)
spec.loader.exec_module(read)
SESSION = "11111111-2222-4333-8444-555555555555"
SESSIONS = ["--server", "http://localhost:7437", "session", "list"]
WORK = ["--server", "http://localhost:7437", "work", "list", "--session", SESSION]


def result(code=0, stdout="ok\n", stderr=""):
    return subprocess.CompletedProcess([], code, stdout.encode("utf-8"), stderr.encode("utf-8"))


class IdeafyReadTest(unittest.TestCase):
    def invoke(self, outcomes, args=SESSIONS, *, jitter=lambda: 0, sleep=None):
        calls, delays = [], []
        out, err = io.BytesIO(), io.BytesIO()
        sequence = iter(outcomes)

        def runner(command, **options):
            calls.append((list(command), options))
            value = next(sequence)
            if isinstance(value, BaseException):
                raise value
            return value

        code = read.main(args, runner=runner, sleep=delays.append if sleep is None else sleep,
                         jitter=jitter, stdout=out, stderr=err)
        return code, out.getvalue().decode("utf-8"), err.getvalue().decode("utf-8"), calls, delays

    def test_U01_immediate_success_preserves_streams_and_options(self):
        code, out, err, calls, delays = self.invoke([result(stdout="ok\r\n", stderr="warning\r\n")])
        self.assertEqual((code, out, err, delays), (0, "ok\r\n", "warning\r\n", []))
        self.assertEqual(calls, [(["you", "--debug", *SESSIONS], dict(capture_output=True, timeout=30))])

    def test_U02_U03_http_recovers_with_original_arguments(self):
        for args, message in [(SESSIONS, "list factory sessions failed (500)"),
                              (WORK, "list work failed (503)")]:
            with self.subTest(args=args):
                code, out, err, calls, delays = self.invoke([result(7, "partial", message), result()], args)
                self.assertEqual((code, out, delays), (0, "ok\n", [1]))
                self.assertEqual([c[0] for c in calls], [["you", "--debug", *args]] * 2)
                self.assertIn("class=http-5xx", err)
                self.assertNotIn(message, err)

    def test_paginated_work_5xx_recovers_with_original_session_and_streams(self):
        for prefix in ("", "Error: ", "debug: cause[0]="):
            with self.subTest(prefix=prefix):
                message = prefix + "work list page 2 failed (503): transient fixture"
                code, out, err, calls, delays = self.invoke(
                    [result(1, "partial page one", message), result(stdout="complete\r\n", stderr="success\r\n")], WORK)
                self.assertEqual((code, out, delays), (0, "complete\r\n", [1]))
                self.assertEqual([c[0] for c in calls], [["you", "--debug", *WORK]] * 2)
                self.assertEqual(err, "ideafy-read: work-list attempt=1/4 class=http-5xx retry-delay=1.000s\nsuccess\r\n")

    def test_paginated_work_4xx_fails_without_retry_despite_misleading_body(self):
        for status in (400, 401, 403, 404):
            with self.subTest(status=status):
                message = (f'debug: cause[0]=work list page 2 failed ({status}): '
                           'timeout 503\nGet "http://localhost/work": i/o timeout\n')
                code, out, err, calls, delays = self.invoke([result(8, "partial page one", message)], WORK)
                self.assertEqual((code, out, len(calls), delays), (8, "", 1, []))
                self.assertEqual(calls[0][0], ["you", "--debug", *WORK])
                self.assertEqual(err, message + "ideafy-read: work-list failed after 1 attempt(s); class=permanent-or-local\n")

    def test_paginated_work_5xx_exhausts_four_attempt_budget(self):
        message = "debug: cause[0]=work list page 12 failed (503): transient fixture\n"
        code, out, err, calls, delays = self.invoke([result(7, "partial page one", message)] * 4, WORK)
        self.assertEqual((code, out, delays), (7, "", [1, 2, 4]))
        self.assertEqual([c[0] for c in calls], [["you", "--debug", *WORK]] * 4)
        self.assertEqual(err.count("retry-delay="), 3)
        self.assertTrue(err.endswith(message + "ideafy-read: work-list failed after 4 attempt(s); class=http-5xx\n"))

    def test_U04_typed_and_transport_timeouts_recover(self):
        errors = [subprocess.TimeoutExpired(["you"], 30)]
        errors += [result(1, "partial", f'Error: Get "http://localhost/work": {text}')
                   for text in ("context deadline exceeded", "Client.Timeout exceeded", "i/o timeout")]
        errors += [result(1, "partial", 'debug: cause[1]=Get "http://localhost/work": i/o timeout')]
        for error in errors:
            with self.subTest(error=error):
                code, out, err, calls, delays = self.invoke([error, result()])
                self.assertEqual((code, out, len(calls), delays), (0, "ok\n", 2, [1]))
                self.assertIn("timeout", err)

    def test_U05_fourth_attempt_and_bounded_jitter(self):
        for jitter, expected in [(lambda: 0.125, [1.125, 2.125, 4.125]),
                                 (lambda: 9, [1.25, 2.25, 4.25]), (lambda: -1, [1, 2, 4])]:
            with self.subTest(expected=expected):
                code, out, err, calls, delays = self.invoke(
                    [result(3, "partial", "list work failed (599)")] * 3 + [result()], WORK, jitter=jitter)
                self.assertEqual((code, out, len(calls), delays), (0, "ok\n", 4, expected))
                self.assertEqual(err.count("retry-delay="), 3)

    def test_U06_exhaustion_preserves_terminal_code_and_discards_stdout(self):
        for failure, expected_code, diagnostic in [
            (result(7, "partial", "list factory sessions failed (500): final"), 7, "final"),
            (subprocess.TimeoutExpired(["you"], 30), 1, "exceeded 30s")]:
            with self.subTest(failure=failure):
                code, out, err, calls, delays = self.invoke([failure] * 4)
                self.assertEqual((code, out, len(calls), delays), (expected_code, "", 4, [1, 2, 4]))
                self.assertIn(diagnostic, err)
                self.assertIn("failed after 4 attempt(s)", err)

    def test_U07_status_precedes_misleading_body(self):
        for status in (400, 401, 403, 404):
            for prefix in ("list factory sessions", "list work"):
                with self.subTest(status=status, prefix=prefix):
                    message = f'{prefix} failed ({status}): Get "http://localhost": i/o timeout; 500'
                    code, out, err, calls, delays = self.invoke([result(8, "partial", message)])
                    self.assertEqual((code, out, len(calls), delays), (8, "", 1, []))
                    self.assertIn(message, err)

    def test_debug_status_recovers_but_generic_envelope_does_not(self):
        envelope = '{"code":"CLI_COMMAND_FAILED","family":"INTERNAL_SERVER_ERROR","message":"command failed"}\n'
        code, out, err, calls, delays = self.invoke([
            result(1, "partial", envelope + "debug: cause[1]=list factory sessions failed (503): transient"), result()])
        self.assertEqual((code, out, len(calls), delays), (0, "ok\n", 2, [1]))
        code, out, err, calls, delays = self.invoke([result(1, "partial", envelope)])
        self.assertEqual((code, out, len(calls), delays), (1, "", 1, []))

    def test_U08_rejects_other_forms_before_execution(self):
        forms = [[], ["--server", "file:///tmp", "session", "list"],
                 ["--server", "http://host:bad", "session", "list"],
                 ["--server", "http://user:secret@host", "session", "list"],
                 [*WORK[:-1], "invalid"], [*SESSIONS, "--all"],
                 [*WORK[:2], "work", "list"], [*WORK[:2], "submit", "batch", "file"]]
        forms += [[*WORK[:2], "work", verb, SESSION, "init"] for verb in ("move", "reset", "restore")]
        for args in forms:
            with self.subTest(args=args):
                code, out, err, calls, delays = self.invoke([], args)
                self.assertEqual((code, out, calls, delays), (2, "", [], []))
                self.assertIn("invalid arguments", err)

    def test_U09_local_unknown_and_decode_errors_fail_immediately(self):
        failures = [FileNotFoundError("you missing"), PermissionError("denied"),
                    UnicodeDecodeError("utf-8", b"\xff", 0, 1, "invalid"),
                    result(9, "partial", "unknown failure: timeout 500"),
                    result(9, "partial", "decode failed: context deadline exceeded"),
                    result(9, "partial", 'decode failed: Get "http://localhost": i/o timeout'),
                    subprocess.CompletedProcess([], 0, b"\xff", b"")]
        for failure in failures:
            with self.subTest(failure=failure):
                code, out, err, calls, delays = self.invoke([failure])
                self.assertEqual((out, len(calls), delays), ("", 1, []))
                self.assertEqual(code, failure.returncode if isinstance(failure, subprocess.CompletedProcess)
                                 and failure.returncode else 1)
                self.assertIn("failed after 1 attempt(s)", err)

    def test_U10_cancellation_stops_execution_and_backoff(self):
        for cancel_in_sleep in (False, True):
            calls, sleeps, cleaned = [], [], []

            def runner(command, **options):
                calls.append(command)
                try:
                    if not cancel_in_sleep:
                        raise KeyboardInterrupt()
                    return result(1, "partial", "list work failed (500)")
                finally:
                    cleaned.append(True)

            def sleep(delay):
                sleeps.append(delay)
                raise KeyboardInterrupt()

            with self.subTest(cancel_in_sleep=cancel_in_sleep), self.assertRaises(KeyboardInterrupt):
                read.main(WORK, runner=runner, sleep=sleep, jitter=lambda: 0,
                          stdout=io.BytesIO(), stderr=io.BytesIO())
            self.assertEqual((len(calls), cleaned, sleeps), (1, [True], [1] if cancel_in_sleep else []))

    def test_U11_empty_success_is_terminal(self):
        for args in (SESSIONS, WORK):
            with self.subTest(args=args):
                code, out, err, calls, delays = self.invoke(
                    [result(1, "partial", "list work failed (503)"), result(stdout="")], args)
                self.assertEqual((code, out, len(calls), delays), (0, "", 2, [1]))


if __name__ == "__main__":
    unittest.main()
