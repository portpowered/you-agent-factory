"""Structured model calls carry exact response contracts, without file access."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import dub_video
from dub_contract import load_translation_response
from dub_grammar import audit_grammar, literal, translation_grammar
from dub_media import CommandFailed


class DubGrammarTests(unittest.TestCase):
    def test_intertoken_whitespace_has_no_unbounded_generation_path(self):
        for grammar in [translation_grammar([{"id": 4}, {"id": 11}], "ko-KR"),
                        audit_grammar({4, 11})]:
            whitespace = next(line for line in grammar.splitlines() if line.startswith("ws ::="))
            self.assertEqual(whitespace, r"ws ::= [ \t\n\r]?")
            self.assertNotIn("ws*", grammar)
            self.assertNotIn("ws+", grammar)
            # Text remains an unconstrained sequence of legal JSON characters.
            self.assertIn('character+', grammar)

    def test_translation_and_focused_audit_forward_distinct_contracts(self):
        source = [{"id": 4, "start": 0, "end": 1000, "text": "你好"}]
        calls = []
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def infer(name, operation, inputs, outputs, parameters=None):
                calls.append(parameters["grammar"])
                prompt = Path(inputs[0].removeprefix("prompt=@")).read_text(encoding="utf-8")
                self.assertNotIn('If returning a file', prompt)
                reply = {"valid": True, "issues": []} if len(calls) == 2 else \
                    {"language": "en-US", "segments": [{"id": 4, "text": "Hello"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer):
                result = dub_video.translate_batch(root, source, "en-US", "llm", 0)
            self.assertEqual(result[0]["source_text"], "你好")
            self.assertEqual(calls, [translation_grammar(source, "en-US"), audit_grammar({4})])
            focused = audit_grammar({7})
            self.assertIn('identifier ::= "7"', focused)
            self.assertNotIn('identifier ::= "4"', focused)
            self.assertIn('"true"', focused)
            self.assertIn('"false"', focused)
            self.assertNotIn('filename', calls[0])

    def test_terminal_quoting_roundtrips_unicode_quotes_and_control_escapes(self):
        for value in ['zh-CN', '韩文 日本語 中文', 'quote"\\\n\t\x00', 59]:
            self.assertEqual(json.loads(json.loads(literal(value))), value)
        grammar = translation_grammar([{"id": 59}], "ko-KR")
        self.assertIn(r'\x00-\x1F', grammar)
        self.assertIn(r'"u" [0-9a-fA-F]{4}', grammar)

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
                calls.append(operation)
                if len(calls) == 2:
                    raise failure
                dub_video.save_json(Path(outputs[0].removeprefix("text=")),
                    {"language": "en-US", "segments": [{"id": 4, "text": "Hello"}]})

            with patch.object(dub_video, "model", side_effect=infer):
                with self.assertRaises(CommandFailed) as caught:
                    dub_video.translate_batch(root, source, "en-US", "llm", 0)
            self.assertIs(caught.exception, failure)
            self.assertEqual(len(calls), 2)


if __name__ == "__main__":
    unittest.main()
