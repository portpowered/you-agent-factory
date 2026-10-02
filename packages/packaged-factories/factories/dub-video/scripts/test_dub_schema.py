"""Structured model calls carry exact JSON Schema contracts, without file access."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import dub_video
from dub_contract import load_translation_response
from dub_media import CommandFailed
from dub_schema import audit_schema, translation_schema


class DubSchemaTests(unittest.TestCase):
    def setUp(self):
        # These tests isolate the existing decision/fit contract. The separate
        # two-stage audit suite exercises the real comparison dispatch.
        comparison = patch.object(dub_video, "compare_translation", return_value="Source comparison evidence")
        comparison.start()
        self.addCleanup(comparison.stop)

    def test_translation_and_focused_audit_forward_distinct_contracts(self):
        source = [{"id": 4, "start": 0, "end": 1000, "text": "你好"}]
        calls = []
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def infer(name, operation, inputs, outputs, parameters=None):
                calls.append(parameters)
                prompt = Path(inputs[0].removeprefix("prompt=@")).read_text(encoding="utf-8")
                self.assertNotIn('If returning a file', prompt)
                reply = {"valid": True, "issues": []} if len(calls) == 2 else \
                    {"language": "en-US", "segments": [{"id": 4, "text": "Hello"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer):
                result = dub_video.translate_batch(root, source, "en-US", "llm", 0)
            self.assertEqual(result[0]["source_text"], "你好")
            # The only forwarded parameter is the schema: thinking stays enabled and
            # no grammar, token, or word-length cap rides along with it.
            self.assertEqual(calls, [{"json_schema": translation_schema(source, "en-US")},
                                     {"json_schema": audit_schema({4})}])
            focused = audit_schema({7})
            self.assertIn(7, focused["oneOf"][1]["properties"]["issues"]["items"]["properties"]["segment_id"]["enum"])
            self.assertNotIn(4, focused["oneOf"][1]["properties"]["issues"]["items"]["properties"]["segment_id"]["enum"])

    def test_translation_schema_is_a_closed_object_of_ordered_cue_constants(self):
        schema = translation_schema([{"id": 4, "start": 0}, {"id": 11, "start": 1000}], "ko-KR")
        self.assertEqual(schema["type"], "object")
        self.assertEqual(schema["required"], ["language", "segments"])
        self.assertFalse(schema["additionalProperties"])
        self.assertEqual(schema["properties"]["language"], {"const": "ko-KR"})
        segments = schema["properties"]["segments"]
        self.assertEqual(segments["type"], "array")
        self.assertEqual([cue["properties"]["id"] for cue in segments["prefixItems"]],
                         [{"const": 4}, {"const": 11}])
        self.assertEqual(segments["minItems"], segments["maxItems"])
        self.assertEqual((segments["minItems"], segments["maxItems"]), (2, 2))
        for cue in segments["prefixItems"]:
            self.assertEqual(cue["required"], ["id", "text"])
            self.assertFalse(cue["additionalProperties"])
            self.assertEqual(cue["properties"]["text"], {"type": "string", "minLength": 1})

    def test_ordered_cue_tuple_never_adds_the_items_keyword(self):
        # The pinned native converter reads prefixItems only when items is absent.
        schema = translation_schema([{"id": 4}, {"id": 11}], "ko-KR")
        segments = schema["properties"]["segments"]
        self.assertIn("prefixItems", segments)
        self.assertNotIn("items", segments)
        self.assertEqual(len(translation_schema([], "ko-KR")["properties"]["segments"]["prefixItems"]), 0)

    def test_audit_schema_is_a_closed_decision_with_at_least_one_indexed_issue(self):
        schema = audit_schema({11, 4, 7})
        self.assertEqual(schema["type"], "object")
        self.assertNotIn("properties", schema)
        approved, rejected = schema["oneOf"]
        for branch in (approved, rejected):
            self.assertEqual(branch["required"], ["valid", "issues"])
            self.assertFalse(branch["additionalProperties"])
        self.assertEqual(approved["properties"]["valid"], {"const": True})
        self.assertEqual(approved["properties"]["issues"], {"const": []})
        self.assertEqual(rejected["properties"]["valid"], {"const": False})
        issues = rejected["properties"]["issues"]
        self.assertEqual((issues["type"], issues["minItems"]), ("array", 1))
        issue = issues["items"]
        self.assertEqual(issue["required"], ["segment_id", "suggested_correction"])
        self.assertFalse(issue["additionalProperties"])
        self.assertEqual(issue["properties"]["segment_id"]["enum"], [4, 7, 11])
        self.assertEqual(issue["properties"]["suggested_correction"],
                         {"type": "string", "minLength": 1})

    def test_no_schema_field_caps_text_tokens_or_word_length(self):
        translation = json.dumps(translation_schema([{"id": 59}], "ko-KR"), ensure_ascii=False)
        audit = json.dumps(audit_schema({59}), ensure_ascii=False)
        for document in (translation, audit):
            for cap in ("maxLength", "maxItems_beyond_cue_count", "pattern", "maxTokens"):
                self.assertNotIn(cap, document)
            self.assertNotIn("enable_thinking", document)
        # Only the cue count is bounded, and only by the exact number of cues.
        self.assertEqual(translation_schema([{"id": index} for index in range(64)], "ko-KR")
                         ["properties"]["segments"]["maxItems"], 64)

    def test_schema_constrains_representation_and_leaves_meaning_to_the_audit(self):
        source = [{"id": 4, "start": 0, "end": 1000, "text": "有没有好心人可以帮帮我？"}]
        calls = []
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def infer(name, operation, inputs, outputs, parameters=None):
                calls.append(parameters)
                path = Path(outputs[0].removeprefix("text="))
                if len(calls) == 2:
                    dub_video.save_json(path, {"valid": True, "issues": []})
                    return
                dub_video.save_json(path, {"language": "en-US",
                                          "segments": [{"id": 4, "text": "Hello"}]})

            with patch.object(dub_video, "model", side_effect=infer):
                # A schema-valid object that drops the explicit 好心 modifier is
                # still accepted structurally: meaning needs the audit, not the schema.
                result = dub_video.translate_batch(root, source, "en-US", "llm", 0)
            self.assertEqual(result[0]["text"], "Hello")
            self.assertEqual(len(calls), 2)

    def test_unicode_and_escaped_text_stay_representable_under_the_schema(self):
        segments = [{"id": 59, "start": 0, "end": 1000, "text": "안녕하세요"}]
        text = "안녕하세요 日本語 中文 \"quoted\"\\ line\nbreak\ttab"
        reply = {"language": "ko-KR", "segments": [{"id": 59, "text": text}]}
        calls = []
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def infer(name, operation, inputs, outputs, parameters=None):
                calls.append(parameters)
                path = Path(outputs[0].removeprefix("text="))
                if len(calls) == 2:
                    dub_video.save_json(path, {"valid": True, "issues": []})
                else:
                    dub_video.save_json(path, reply)

            with patch.object(dub_video, "model", side_effect=infer):
                result = dub_video.translate_batch(root, segments, "ko-KR", "llm", 0)
            self.assertEqual(result[0]["text"], text)
        # The forwarded schema survives a Unicode-safe JSON round trip unchanged.
        schema = calls[0]["json_schema"]
        self.assertEqual(json.loads(json.dumps(schema, ensure_ascii=False)), schema)
        self.assertEqual(schema["properties"]["language"], {"const": "ko-KR"})

    def test_plain_model_response_cannot_follow_a_file_pointer(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "translation.json"
            path.write_text('{"language":"en-US","segments":[]}', encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "not a filename"):
                load_translation_response(json.dumps({"filename": str(path)}))

    def test_audit_model_cancellation_propagates_without_regenerating_batch(self):
        source = [{"id": 4, "start": 0, "end": 1000, "text": "你好"}]
        calls = []
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            failure = CommandFailed("you", 130, "caller canceled")

            def infer(name, operation, inputs, outputs, parameters=None):
                calls.append((operation, parameters))
                if len(calls) == 2:
                    raise failure
                dub_video.save_json(Path(outputs[0].removeprefix("text=")),
                    {"language": "en-US", "segments": [{"id": 4, "text": "Hello"}]})

            with patch.object(dub_video, "model", side_effect=infer):
                with self.assertRaises(CommandFailed) as caught:
                    dub_video.translate_batch(root, source, "en-US", "llm", 0)
            self.assertIs(caught.exception, failure)
            self.assertEqual(len(calls), 2)
            self.assertEqual([operation for operation, _ in calls], ["OMNI", "OMNI"])
            self.assertEqual(calls[1][1], {"json_schema": audit_schema({4})})


if __name__ == "__main__":
    unittest.main()
