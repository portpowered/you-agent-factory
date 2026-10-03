"""Fit review targets changed cues; neighboring source stays context only."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import dub_video


class DubAuditFocusTests(unittest.TestCase):
    def setUp(self):
        # These tests isolate the existing decision/fit contract. The separate
        # two-stage audit suite exercises the real comparison dispatch.
        comparison = patch.object(dub_video, "compare_translation", return_value="Source comparison evidence")
        comparison.start()
        self.addCleanup(comparison.stop)

    def segments(self):
        return [{"id": 0, "source_text": "你好", "text": "Hello"},
                {"id": 1, "source_text": "有没有好心人可以帮帮我？", "text": "Can any kind person help me?"},
                {"id": 2, "source_text": "拜托拜托", "text": "Please, please"}]

    def test_concise_question_review_declares_focus_and_preserves_neighbor_context(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def infer(name, operation, inputs, outputs, parameters=None):
                prompt = Path(inputs[0].removeprefix("prompt=@")).read_text(encoding="utf-8")
                request = json.loads(prompt.splitlines()[-1])
                self.assertEqual(request["focus_ids"], [1])
                self.assertEqual([item["id"] for item in request["segments"]], [0, 1, 2])
                self.assertEqual(request["segments"][1]["source"], "有没有好心人可以帮帮我？")
                self.assertIn("'Can any kind person help me?'", prompt)
                self.assertIn("'Is there anyone kind who can help me?'", prompt)
                self.assertIn("Audit ONLY focus_ids", prompt)
                Path(outputs[0].removeprefix("text=")).write_text('{"valid":true,"issues":[]}', encoding="utf-8")

            with patch.object(dub_video, "model", side_effect=infer):
                dub_video.audit_translation(root, self.segments(), "en-US", "llm", "-fit", 1, focus_ids=[1])

    def test_focused_issue_must_be_structured_and_identify_changed_source_cue(self):
        replies = [
            {"valid": False, "issues": [{"segment_id": 0, "suggested_correction": "Hi"}]},
            {"valid": False, "issues": ["Segment 1 is wrong"]},
            {"valid": False, "issues": [{"segment_id": True, "suggested_correction": "Help me"}]},
            {"valid": False, "issues": [{"segment_id": 1, "suggested_correction": "Help me", "extra": 0}]},
        ]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for reply in replies:
                def infer(name, operation, inputs, outputs, parameters=None):
                    dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)
                with self.subTest(reply=reply), patch.object(dub_video, "model", side_effect=infer):
                    with self.assertRaises(ValueError) as rejected:
                        dub_video.audit_translation(root, self.segments(), "en-US", "llm", "-fit", 1, focus_ids=[1])
                    self.assertNotIsInstance(rejected.exception, dub_video.TranslationAuditRejected)

    def test_actual_focused_semantic_error_remains_rejection(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            segments = self.segments()
            segments[1]["text"] = "I can help you."

            def infer(name, operation, inputs, outputs, parameters=None):
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), {"valid": False, "issues": [
                    {"segment_id": 1, "suggested_correction": "Can any kind person help me?"}]})

            with patch.object(dub_video, "model", side_effect=infer):
                with self.assertRaises(dub_video.TranslationAuditRejected) as rejected:
                    dub_video.audit_translation(root, segments, "en-US", "llm", "-fit", 1, focus_ids=[1])
            self.assertEqual(rejected.exception.corrections, {1: "Can any kind person help me?"})

    def test_invalid_focus_is_rejected_before_model_invocation(self):
        with tempfile.TemporaryDirectory() as directory:
            for focus in ([], [True], [3], [1, 1], "1"):
                with self.subTest(focus=focus), patch.object(dub_video, "model") as infer:
                    with self.assertRaisesRegex(ValueError, "focus IDs"):
                        dub_video.audit_translation(Path(directory), self.segments(), "en-US", "llm", "-fit", 1,
                                                    focus_ids=focus)
                    infer.assert_not_called()


if __name__ == "__main__":
    unittest.main()
