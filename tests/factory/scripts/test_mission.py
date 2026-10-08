"""Isolated routing and reply validation with controlled JSON values."""

import importlib.util
import json
from pathlib import Path
import unittest


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


class MissionOutputTests(unittest.TestCase):
    def valid(self, value=0):
        return {"decision": "ACCEPTED", "feedback": "measured", "output": {
            "measurements": [{"name": "pending", "value": value, "source": "you work list"}]}}

    def test_native_values_and_evidence_preserved(self):
        for value in (0, False, None, "", {"count": 0}, [1, 2]):
            reply = self.valid(value)
            reply["output"].update(receipt="request-1", proposal="path", reads=[{"attempts": 2}])
            self.assertEqual(checker.check_mission_output(json.dumps(reply)), reply)

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


if __name__ == "__main__":
    unittest.main()
