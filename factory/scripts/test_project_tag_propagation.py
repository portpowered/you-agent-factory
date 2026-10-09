"""Component tests for the tag-lineage static checker."""

import importlib.util
import contextlib
import io
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("tag_lint", Path(__file__).with_name("lint-project-tag-propagation.py"))
lint = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lint)


def edge(kind):
    return {"workType": kind, "state": "ready"}


def fixture(inputs=None, outputs=None, mode=None):
    station = {"id": "escalate", "inputs": inputs if inputs is not None else [edge("project")], "outputs": outputs if outputs is not None else [edge("thoughts")]}
    if mode is not None:
        station["workPropagation"] = {"mode": mode}
    return {"resources": [{"name": "slot"}], "workTypes": [{"name": kind} for kind in ("project", "thoughts")], "workstations": [station]}


class TagPropagationTests(unittest.TestCase):
    def test_same_type_clones_matching_input(self):
        for mode in (None, "OUTPUT_AS_PAYLOAD", "PRESERVE_INPUT"):
            with self.subTest(mode=mode):
                self.assertEqual(lint.check(fixture(outputs=[edge("project")], mode=mode)), ([], 1))

    def test_cross_type_requires_preservation(self):
        findings, count = lint.check(fixture())
        self.assertEqual(count, 1)
        self.assertIn("escalate.outputs[0] (thoughts)", findings[0])
        self.assertEqual(lint.check(fixture(mode="PRESERVE_INPUT")), ([], 1))

    def test_all_outcomes_and_classification_routes(self):
        for outcome in ("onFailure", "onRejection", "onContinue", "classificationRoutes"):
            with self.subTest(outcome=outcome):
                document = fixture(outputs=[edge("project")])
                document["workstations"][0][outcome] = ([{"label": "failed", "outputs": [edge("thoughts")]}] if outcome == "classificationRoutes" else [edge("thoughts")])
                findings, count = lint.check(document)
                self.assertEqual(count, 2)
                self.assertEqual(len(findings), 1)
                self.assertIn(outcome, findings[0])

    def test_resource_only_is_not_a_source(self):
        self.assertEqual(lint.check(fixture(inputs=[])), ([], 1))
        self.assertTrue(lint.check(fixture(inputs=[edge("slot")], mode="PRESERVE_INPUT"))[0])
        self.assertEqual(lint.check(fixture(inputs=[edge("slot")], outputs=[edge("slot")])), ([], 0))

    def test_multiple_inputs_with_target_match(self):
        self.assertEqual(lint.check(fixture(inputs=[edge("slot"), edge("thoughts"), edge("project")], outputs=[edge("project")], mode="OUTPUT_AS_PAYLOAD")), ([], 1))
        self.assertEqual(lint.check(fixture(inputs=[edge("slot"), edge("project")], mode="PRESERVE_INPUT")), ([], 1))

    def test_malformed_metadata_fails_closed(self):
        for value in (None, {}, {"workstations": []}, fixture(mode="invented"), fixture(outputs=[{}]), fixture(inputs=[edge("unknown")])):
            with self.subTest(value=value), self.assertRaises(ValueError):
                lint.check(value)
        for key in ("onFailure", "classificationRoutes", "workPropagation"):
            value = fixture()
            value["workstations"][0][key] = "invalid"
            with self.subTest(key=key), self.assertRaises(ValueError):
                lint.check(value)

    def test_malformed_json_command_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "factory.json"
            path.write_text("{invalid", encoding="utf-8")
            diagnostic = io.StringIO()
            with contextlib.redirect_stderr(diagnostic):
                self.assertEqual(lint.main([str(path)]), 1)
            self.assertIn("Project tag propagation:", diagnostic.getvalue())


if __name__ == "__main__":
    unittest.main()
