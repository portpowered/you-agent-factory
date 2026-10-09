"""One read operation with a controlled subprocess boundary, no live reads."""

import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import unittest
from unittest.mock import Mock

path = Path(__file__).resolve().parents[3] / "factory/scripts/mission-read.py"
spec = importlib.util.spec_from_file_location("mission_read", path)
reader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reader)


class MissionReadTests(unittest.TestCase):
    def test_success_uses_argv_without_shell_once(self):
        command = ["gh", "run", "view", "123", "--json", "status"]
        run = Mock(return_value=subprocess.CompletedProcess(command, 0, "zero=0", ""))
        result, code = reader.mission_read(command, run=run)
        self.assertEqual(code, 0)
        self.assertTrue(result["available"])
        self.assertEqual(result["value"], "zero=0")
        self.assertEqual(result["attempts"], 1)
        run.assert_called_once_with(command, capture_output=True, text=True, encoding="utf-8",
                                    errors="replace", timeout=30, shell=False)

    def test_non_ascii_output_is_read_without_the_locale_codec(self):
        script = "import sys; sys.stdout.buffer.write('“ok” \x9d'.encode('utf-8'))"
        import sys as _sys
        result, code = reader.mission_read([_sys.executable, "-c", script])
        self.assertEqual(code, 0)
        self.assertTrue(result["value"].startswith("“ok”"))

    def test_retry_success_retains_first_partial_output(self):
        run = Mock(side_effect=[subprocess.CompletedProcess([], 1, "partial", "secret error body"),
                                subprocess.CompletedProcess([], 0, "available", "")])
        result, code = reader.mission_read(["read"], run=run)
        self.assertEqual((code, result["attempts"], run.call_count), (0, 2, 2))
        self.assertEqual([record["value"] for record in result["records"]], ["partial", "available"])
        self.assertNotIn("secret error body", json.dumps(result))

    def test_optional_and_required_exhaustion(self):
        for required in (False, True):
            for fault in (subprocess.CompletedProcess([], 5, "partial", "private"),
                          subprocess.TimeoutExpired("read", 30, output=b"partial"),
                          FileNotFoundError("private path")):
                with self.subTest(required=required, fault=fault):
                    run = Mock(side_effect=[fault, fault] if isinstance(fault, Exception) else None,
                               return_value=fault)
                    result, code = reader.mission_read(["read"], required, run)
                    self.assertEqual((code, result["attempts"], run.call_count), (int(required), 2, 2))
                    self.assertFalse(result["available"])
                    self.assertEqual(result["required"], required)
                    self.assertTrue(result["error"])
                    self.assertNotIn("private", json.dumps(result))

    def test_cancellation_never_retries(self):
        run = Mock(side_effect=KeyboardInterrupt)
        result, code = reader.mission_read(["read"], run=run)
        self.assertEqual((code, run.call_count, result["attempts"]), (130, 1, 1))
        self.assertTrue(result["cancelled"])

    def test_process_interrupt_preserves_output_without_retry(self):
        for status in (-2, 130, 0xC000013A):
            run = Mock(return_value=subprocess.CompletedProcess([], status, "partial", ""))
            result, code = reader.mission_read(["read"], run=run)
            self.assertEqual((code, run.call_count, result["attempts"]), (130, 1, 1))
            self.assertEqual(result["records"][0]["value"], "partial")

    def test_invalid_cli_does_not_execute(self):
        run = Mock()
        for args in ([], ["read"], ["--"], ["--required", "--"], ["--", " "], ["--unknown", "--", "read"]):
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(reader.main(args, run), 2)
        run.assert_not_called()

    def test_cli_required_failure_emits_attempt_record(self):
        run = Mock(return_value=subprocess.CompletedProcess([], 1, "partial", ""))
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(reader.main(["--required", "--", "read", "a b"], run), 1)
        result = json.loads(output.getvalue())
        self.assertEqual(result["attempts"], 2)
        self.assertTrue(result["required"])
        self.assertEqual(result["records"][0]["value"], "partial")


if __name__ == "__main__":
    unittest.main()
