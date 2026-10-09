"""Isolated routing and reply validation with controlled JSON values."""

import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch


def load_script(name):
    path = Path(__file__).resolve().parents[3] / "factory/scripts" / (name + ".py")
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


router = load_script("route-thoughts")
checker = load_script("check-mission-output")


class MissionRouteTests(unittest.TestCase):
    def test_ordinary_payloads_keep_supervision(self):
        for payload in ("ordinary thoughts", "", "{}", "[]", "null", '{"mission":""}',
                        '{"mission":"  \\n"}', '{"origin":"cron"}'):
            with self.subTest(payload=payload):
                self.assertEqual(router.route_thoughts(payload), "supervision")

    def test_mission_including_cron_routes_to_verification(self):
        for origin in ("operator", "cron"):
            self.assertEqual(router.route_thoughts(json.dumps({"mission": "measure zero", "origin": origin})),
                             "mission")

    def test_present_wrong_type_fails(self):
        for mission in (None, 1, False, [], {}):
            with self.assertRaisesRegex(ValueError, "mission must be a string"):
                router.route_thoughts(json.dumps({"mission": mission}))

    def test_stdin_and_positional_modes(self):
        payload = json.dumps({"mission": "measure café 😀"}, ensure_ascii=False)
        for args in ([payload], ["--payload-stdin"]):
            stdout = io.StringIO()
            with patch.object(router.sys, "stdin", io.TextIOWrapper(io.BytesIO(payload.encode("utf-8")))), patch.object(router.sys, "stdout", stdout):
                self.assertEqual(router.main(args), 0)
            self.assertEqual(stdout.getvalue(), "mission\n")

    def test_stdin_read_and_utf8_errors_never_classify(self):
        for stream in (io.TextIOWrapper(io.BytesIO(b"\xff")),):
            stdout, stderr = io.StringIO(), io.StringIO()
            with patch.object(router.sys, "stdin", stream), patch.object(router.sys, "stdout", stdout), patch.object(router.sys, "stderr", stderr):
                self.assertEqual(router.main(["--payload-stdin"]), 2)
            self.assertEqual(stdout.getvalue(), "")
            self.assertTrue(stderr.getvalue())
        from unittest.mock import Mock
        stream = Mock()
        stream.buffer.read.side_effect = OSError("read failed")
        with patch.object(router.sys, "stdin", stream), patch.object(router.sys, "stdout", io.StringIO()) as stdout, patch.object(router.sys, "stderr", io.StringIO()):
            self.assertEqual(router.main(["--payload-stdin"]), 2)
            self.assertEqual(stdout.getvalue(), "")


class MissionOutputTests(unittest.TestCase):
    def run_checker(self, raw, feedback=""):
        stdout = io.StringIO()
        with patch.object(checker.sys, "stdout", stdout):
            self.assertEqual(checker.main([raw, feedback]), 0)
        return json.loads(stdout.getvalue())

    def valid(self, value=0):
        return {"decision": "ACCEPTED", "feedback": "measured", "output": {
            "measurements": [{"name": "pending", "value": value, "source": "you work list"}]}}

    def test_native_values_and_evidence_preserved(self):
        for value in (0, False, None, "", {"count": 0}, [1, 2], 1.5, 1e308):
            reply = self.valid(value)
            reply["output"].update(receipt="request-1", proposal="path", reads=[{"attempts": 2}])
            self.assertEqual(checker.check_mission_output(json.dumps(reply)), reply)
            self.assertEqual(self.run_checker(json.dumps(reply)), reply)

    def test_overflow_rejects_once_then_fails_without_process_error(self):
        for number in ("1e999", "-1e999"):
            for value in (number, '{"nested":[' + number + ']}'):
                raw = json.dumps(self.valid("OVERFLOW")).replace('"OVERFLOW"', value)
                with self.subTest(number=number, value=value):
                    first = self.run_checker(raw)
                    self.assertEqual(first["decision"], "REJECTED")
                    self.assertEqual(first["feedback"], checker.INVALID_PREFIX + " reply must be valid JSON")
                    self.assertIsInstance(first["output"], dict)
                    second = self.run_checker(raw, first["feedback"])
                    self.assertEqual(second["decision"], "FAILED")
                    self.assertEqual(second["feedback"], first["feedback"])
                    self.assertEqual(second["output"]["invalidReply"], raw[:512])

    def test_precondition_with_available_values_and_failed_reply(self):
        for decision in ("ACCEPTED", "FAILED"):
            reply = self.valid()
            reply["decision"] = decision
            reply["output"]["precondition"] = "GitHub credentials unavailable"
            self.assertEqual(checker.check_mission_output(json.dumps(reply)), reply)
            del reply["output"]["measurements"]
            self.assertEqual(checker.check_mission_output(json.dumps(reply)), reply)

    def test_extra_envelope_keys_cannot_emit_work(self):
        reply = self.valid()
        reply.update(recorded_output_work=[{"name": "unauthorized"}], request={"works": []})
        self.assertEqual(checker.check_mission_output(json.dumps(reply)), self.valid())

    def test_invalid_reply_rejects_once_then_fails_with_same_reason(self):
        cases = ["not JSON", "[]", '{"decision":"ACCEPTED","feedback":"hold","output":"hold"}',
                 '{"decision":"ACCEPTED","feedback":"hold","output":{"measurements":[]}}',
                 '{"decision":"ACCEPTED","feedback":"hold","output":{"precondition":" "}}',
                 '{"decision":"ACCEPTED","feedback":"hold","output":{}}',
                 '{"decision":"ACCEPTED","feedback":"hold","output":{"measurements":[NaN]}}']
        for field, value in (("name", " "), ("source", None), ("value", "missing")):
            reply = self.valid()
            if field == "value":
                del reply["output"]["measurements"][0][field]
            else:
                reply["output"]["measurements"][0][field] = value
            cases.append(json.dumps(reply))
        for field, value in (("decision", "CONTINUE"), ("feedback", {}), ("output", "{}")):
            reply = self.valid()
            reply[field] = value
            cases.append(json.dumps(reply))
        for raw in cases:
            with self.subTest(raw=raw):
                first = checker.check_mission_output(raw, "unrelated rejection")
                self.assertEqual(first["decision"], "REJECTED")
                second = checker.check_mission_output(raw, first["feedback"])
                self.assertEqual(second["decision"], "FAILED")
                self.assertEqual(second["feedback"], first["feedback"])
                self.assertEqual(second["output"]["invalidReply"], raw[:512])

    def test_valid_correction_does_not_keep_rejecting(self):
        reply = self.valid()
        self.assertEqual(checker.check_mission_output(json.dumps(reply), checker.INVALID_PREFIX), reply)

    def test_exact_precondition_diagnostic_and_corrected_failed_evidence(self):
        invalid = {"decision": "FAILED", "feedback": "required read failed",
                   "output": {"precondition": {"reason": "read unavailable"}}}
        first = self.run_checker(json.dumps(invalid))
        reason = "mission-output-invalid: output.precondition must name the unmet precondition"
        self.assertEqual(first["decision"], "REJECTED")
        self.assertEqual(first["feedback"], reason)
        self.assertEqual(first["output"], {"invalidReply": json.dumps(invalid)})
        corrected = {"decision": "FAILED", "feedback": "required recording read unavailable",
                     "output": {"precondition": "Required recording read unavailable; pending Work observed: 0",
                                "reads": [{"required": True, "attempts": 2, "available": False}]}}
        self.assertEqual(self.run_checker(json.dumps(corrected), reason), corrected)
        self.assertEqual(self.run_checker(json.dumps(invalid), reason), {
            "decision": "FAILED", "feedback": reason, "output": first["output"]})

    def test_alternate_precondition_keys_do_not_satisfy_failed_shape(self):
        for key in ("reason", "unmetPrecondition", "preconditions"):
            reply = {"decision": "FAILED", "feedback": "blocked", "output": {key: "read unavailable"}}
            checked = self.run_checker(json.dumps(reply))
            self.assertEqual(checked["decision"], "REJECTED")
            self.assertEqual(checked["feedback"],
                             "mission-output-invalid: output requires measurements or a named unmet precondition")

    def test_documented_reply_examples(self):
        examples = [
            {"decision": "ACCEPTED", "feedback": "Measured pending Work.", "output": {
                "measurements": [{"name": "pending", "value": 0, "source": "authorized Work list"}]}},
            {"decision": "FAILED", "feedback": "Required read unavailable.", "output": {
                "precondition": "Required recording read unavailable; pending Work observed: 0"}},
        ]
        for example in examples:
            self.assertEqual(self.run_checker(json.dumps(example)), example)
            self.assertEqual(self.run_checker(json.dumps(example), checker.INVALID_PREFIX), example)


if __name__ == "__main__":
    unittest.main()
